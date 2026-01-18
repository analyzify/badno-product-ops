package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/badno/badops/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CompetitorRepo implements the CompetitorRepository interface for PostgreSQL
type CompetitorRepo struct {
	client *Client
}

// NewCompetitorRepo creates a new PostgreSQL competitor repository
func NewCompetitorRepo(client *Client) *CompetitorRepo {
	return &CompetitorRepo{client: client}
}

// normalizeCompetitorName creates a normalized version of the competitor name
func normalizeCompetitorName(name string) string {
	normalized := strings.ToLower(name)
	normalized = strings.ReplaceAll(normalized, " ", "_")
	normalized = strings.ReplaceAll(normalized, ".", "")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}

// Create inserts a new competitor into the database
func (r *CompetitorRepo) Create(ctx context.Context, competitor *database.Competitor) error {
	if competitor.NormalizedName == "" {
		competitor.NormalizedName = normalizeCompetitorName(competitor.Name)
	}

	query := `
		INSERT INTO competitors (name, normalized_name, website, scrape_enabled, scrape_config, product_count)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	configJSON, _ := json.Marshal(competitor.ScrapeConfig)

	err := r.client.pool.QueryRow(ctx, query,
		competitor.Name,
		competitor.NormalizedName,
		competitor.Website,
		competitor.ScrapeEnabled,
		configJSON,
		competitor.ProductCount,
	).Scan(&competitor.ID, &competitor.CreatedAt, &competitor.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create competitor: %w", err)
	}

	return nil
}

// GetByID retrieves a competitor by its ID
func (r *CompetitorRepo) GetByID(ctx context.Context, id int) (*database.Competitor, error) {
	query := `
		SELECT id, name, normalized_name, website, scrape_enabled, scrape_config,
		       product_count, last_scraped, created_at, updated_at
		FROM competitors
		WHERE id = $1
	`

	row := r.client.pool.QueryRow(ctx, query, id)
	return r.scanCompetitor(row)
}

// GetByName retrieves a competitor by name (case-insensitive)
func (r *CompetitorRepo) GetByName(ctx context.Context, name string) (*database.Competitor, error) {
	normalized := normalizeCompetitorName(name)
	query := `
		SELECT id, name, normalized_name, website, scrape_enabled, scrape_config,
		       product_count, last_scraped, created_at, updated_at
		FROM competitors
		WHERE normalized_name = $1
	`

	row := r.client.pool.QueryRow(ctx, query, normalized)
	return r.scanCompetitor(row)
}

func (r *CompetitorRepo) scanCompetitor(row pgx.Row) (*database.Competitor, error) {
	var c database.Competitor
	var configJSON []byte

	err := row.Scan(
		&c.ID, &c.Name, &c.NormalizedName, &c.Website, &c.ScrapeEnabled, &configJSON,
		&c.ProductCount, &c.LastScraped, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan competitor: %w", err)
	}

	if len(configJSON) > 0 {
		json.Unmarshal(configJSON, &c.ScrapeConfig)
	}

	return &c, nil
}

// GetAll retrieves all competitors
func (r *CompetitorRepo) GetAll(ctx context.Context) ([]*database.Competitor, error) {
	query := `
		SELECT id, name, normalized_name, website, scrape_enabled, scrape_config,
		       product_count, last_scraped, created_at, updated_at
		FROM competitors
		ORDER BY product_count DESC, name
	`

	rows, err := r.client.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query competitors: %w", err)
	}
	defer rows.Close()

	var competitors []*database.Competitor
	for rows.Next() {
		var c database.Competitor
		var configJSON []byte

		err := rows.Scan(
			&c.ID, &c.Name, &c.NormalizedName, &c.Website, &c.ScrapeEnabled, &configJSON,
			&c.ProductCount, &c.LastScraped, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan competitor: %w", err)
		}

		if len(configJSON) > 0 {
			json.Unmarshal(configJSON, &c.ScrapeConfig)
		}

		competitors = append(competitors, &c)
	}

	return competitors, rows.Err()
}

// Update updates an existing competitor
func (r *CompetitorRepo) Update(ctx context.Context, competitor *database.Competitor) error {
	competitor.NormalizedName = normalizeCompetitorName(competitor.Name)

	query := `
		UPDATE competitors SET
			name = $2, normalized_name = $3, website = $4,
			scrape_enabled = $5, scrape_config = $6,
			product_count = $7, last_scraped = $8
		WHERE id = $1
	`

	configJSON, _ := json.Marshal(competitor.ScrapeConfig)

	_, err := r.client.pool.Exec(ctx, query,
		competitor.ID, competitor.Name, competitor.NormalizedName, competitor.Website,
		competitor.ScrapeEnabled, configJSON,
		competitor.ProductCount, competitor.LastScraped,
	)

	if err != nil {
		return fmt.Errorf("failed to update competitor: %w", err)
	}

	return nil
}

// Delete removes a competitor from the database
func (r *CompetitorRepo) Delete(ctx context.Context, id int) error {
	_, err := r.client.pool.Exec(ctx, "DELETE FROM competitors WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete competitor: %w", err)
	}
	return nil
}

// Count returns the total number of competitors
func (r *CompetitorRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.client.pool.QueryRow(ctx, "SELECT COUNT(*) FROM competitors").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count competitors: %w", err)
	}
	return count, nil
}

// GetOrCreate gets an existing competitor by name or creates a new one
func (r *CompetitorRepo) GetOrCreate(ctx context.Context, name string) (*database.Competitor, error) {
	existing, err := r.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	competitor := &database.Competitor{
		Name:          name,
		ScrapeEnabled: false,
	}
	if err := r.Create(ctx, competitor); err != nil {
		return nil, err
	}
	return competitor, nil
}

// CompetitorProductRepo implements the CompetitorProductRepository interface
type CompetitorProductRepo struct {
	client *Client
}

// NewCompetitorProductRepo creates a new PostgreSQL competitor product repository
func NewCompetitorProductRepo(client *Client) *CompetitorProductRepo {
	return &CompetitorProductRepo{client: client}
}

// Create inserts a new competitor product link
func (r *CompetitorProductRepo) Create(ctx context.Context, link *database.CompetitorProduct) error {
	query := `
		INSERT INTO competitor_products (
			product_id, competitor_id, url, competitor_sku, competitor_title,
			is_active, match_method, match_confidence
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (product_id, competitor_id) DO UPDATE SET
			url = EXCLUDED.url,
			competitor_sku = COALESCE(NULLIF(EXCLUDED.competitor_sku, ''), competitor_products.competitor_sku),
			competitor_title = COALESCE(NULLIF(EXCLUDED.competitor_title, ''), competitor_products.competitor_title),
			is_active = EXCLUDED.is_active,
			match_method = EXCLUDED.match_method,
			match_confidence = EXCLUDED.match_confidence
	`

	_, err := r.client.pool.Exec(ctx, query,
		link.ProductID.String(), link.CompetitorID, link.URL, link.CompetitorSKU, link.CompetitorTitle,
		link.IsActive, link.MatchMethod, link.MatchConfidence,
	)

	if err != nil {
		return fmt.Errorf("failed to create competitor product: %w", err)
	}

	return nil
}

// GetByProductAndCompetitor retrieves a specific link
func (r *CompetitorProductRepo) GetByProductAndCompetitor(ctx context.Context, productID uuid.UUID, competitorID int) (*database.CompetitorProduct, error) {
	query := `
		SELECT product_id, competitor_id, url, competitor_sku, competitor_title,
		       is_active, match_method, match_confidence, created_at, updated_at
		FROM competitor_products
		WHERE product_id = $1 AND competitor_id = $2
	`

	var link database.CompetitorProduct
	var productIDStr string

	err := r.client.pool.QueryRow(ctx, query, productID.String(), competitorID).Scan(
		&productIDStr, &link.CompetitorID, &link.URL, &link.CompetitorSKU, &link.CompetitorTitle,
		&link.IsActive, &link.MatchMethod, &link.MatchConfidence, &link.CreatedAt, &link.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get competitor product: %w", err)
	}

	link.ProductID, _ = uuid.Parse(productIDStr)
	return &link, nil
}

// GetByProduct retrieves all competitor links for a product
func (r *CompetitorProductRepo) GetByProduct(ctx context.Context, productID uuid.UUID) ([]*database.CompetitorProduct, error) {
	query := `
		SELECT product_id, competitor_id, url, competitor_sku, competitor_title,
		       is_active, match_method, match_confidence, created_at, updated_at
		FROM competitor_products
		WHERE product_id = $1
	`

	rows, err := r.client.pool.Query(ctx, query, productID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query competitor products: %w", err)
	}
	defer rows.Close()

	return r.scanCompetitorProducts(rows)
}

// GetByCompetitor retrieves all product links for a competitor
func (r *CompetitorProductRepo) GetByCompetitor(ctx context.Context, competitorID int) ([]*database.CompetitorProduct, error) {
	query := `
		SELECT product_id, competitor_id, url, competitor_sku, competitor_title,
		       is_active, match_method, match_confidence, created_at, updated_at
		FROM competitor_products
		WHERE competitor_id = $1
	`

	rows, err := r.client.pool.Query(ctx, query, competitorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query competitor products: %w", err)
	}
	defer rows.Close()

	return r.scanCompetitorProducts(rows)
}

func (r *CompetitorProductRepo) scanCompetitorProducts(rows pgx.Rows) ([]*database.CompetitorProduct, error) {
	var links []*database.CompetitorProduct

	for rows.Next() {
		var link database.CompetitorProduct
		var productIDStr string

		err := rows.Scan(
			&productIDStr, &link.CompetitorID, &link.URL, &link.CompetitorSKU, &link.CompetitorTitle,
			&link.IsActive, &link.MatchMethod, &link.MatchConfidence, &link.CreatedAt, &link.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan competitor product: %w", err)
		}

		link.ProductID, _ = uuid.Parse(productIDStr)
		links = append(links, &link)
	}

	return links, rows.Err()
}

// BulkUpsert inserts or updates multiple competitor product links
func (r *CompetitorProductRepo) BulkUpsert(ctx context.Context, links []*database.CompetitorProduct) (int, error) {
	if len(links) == 0 {
		return 0, nil
	}

	tx, err := r.client.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO competitor_products (
			product_id, competitor_id, url, competitor_sku, competitor_title,
			is_active, match_method, match_confidence
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (product_id, competitor_id) DO UPDATE SET
			url = EXCLUDED.url,
			competitor_sku = COALESCE(NULLIF(EXCLUDED.competitor_sku, ''), competitor_products.competitor_sku),
			competitor_title = COALESCE(NULLIF(EXCLUDED.competitor_title, ''), competitor_products.competitor_title),
			is_active = EXCLUDED.is_active,
			match_method = EXCLUDED.match_method,
			match_confidence = EXCLUDED.match_confidence
	`

	batch := &pgx.Batch{}
	for _, link := range links {
		batch.Queue(query,
			link.ProductID.String(), link.CompetitorID, link.URL, link.CompetitorSKU, link.CompetitorTitle,
			link.IsActive, link.MatchMethod, link.MatchConfidence,
		)
	}

	br := tx.SendBatch(ctx, batch)
	defer br.Close()

	count := 0
	for range links {
		_, err := br.Exec()
		if err != nil {
			return count, fmt.Errorf("failed to upsert competitor product: %w", err)
		}
		count++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return count, nil
}

// Count returns the total number of competitor product links
func (r *CompetitorProductRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.client.pool.QueryRow(ctx, "SELECT COUNT(*) FROM competitor_products").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count competitor products: %w", err)
	}
	return count, nil
}

// CountByCompetitor returns link counts grouped by competitor
func (r *CompetitorProductRepo) CountByCompetitor(ctx context.Context) (map[int]int64, error) {
	query := `SELECT competitor_id, COUNT(*) FROM competitor_products GROUP BY competitor_id`
	rows, err := r.client.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count by competitor: %w", err)
	}
	defer rows.Close()

	counts := make(map[int]int64)
	for rows.Next() {
		var competitorID int
		var count int64
		if err := rows.Scan(&competitorID, &count); err != nil {
			return nil, err
		}
		counts[competitorID] = count
	}
	return counts, rows.Err()
}

// UpdateCompetitorProductCounts updates the product_count field for all competitors
func (r *CompetitorProductRepo) UpdateCompetitorProductCounts(ctx context.Context) error {
	query := `
		UPDATE competitors c SET
			product_count = COALESCE((
				SELECT COUNT(*) FROM competitor_products cp WHERE cp.competitor_id = c.id
			), 0)
	`

	_, err := r.client.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update competitor product counts: %w", err)
	}

	return nil
}

// PriceObservationRepo implements the PriceObservationRepository interface
type PriceObservationRepo struct {
	client *Client
}

// NewPriceObservationRepo creates a new PostgreSQL price observation repository
func NewPriceObservationRepo(client *Client) *PriceObservationRepo {
	return &PriceObservationRepo{client: client}
}

// Create inserts a new price observation
func (r *PriceObservationRepo) Create(ctx context.Context, observation *database.PriceObservation) error {
	query := `
		INSERT INTO price_observations (
			product_id, competitor_id, price, currency, in_stock, stock_quantity,
			observed_at, observed_date, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`

	observedDate := observation.ObservedAt.Truncate(24 * time.Hour)

	err := r.client.pool.QueryRow(ctx, query,
		observation.ProductID.String(),
		observation.CompetitorID,
		observation.Price,
		observation.Currency,
		observation.InStock,
		observation.StockQuantity,
		observation.ObservedAt,
		observedDate,
		observation.Source,
	).Scan(&observation.ID)

	if err != nil {
		return fmt.Errorf("failed to create price observation: %w", err)
	}

	return nil
}

// BulkCreate inserts multiple price observations
func (r *PriceObservationRepo) BulkCreate(ctx context.Context, observations []*database.PriceObservation) (int, error) {
	if len(observations) == 0 {
		return 0, nil
	}

	tx, err := r.client.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO price_observations (
			product_id, competitor_id, price, currency, in_stock, stock_quantity,
			observed_at, observed_date, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	batch := &pgx.Batch{}
	for _, obs := range observations {
		observedDate := obs.ObservedAt.Truncate(24 * time.Hour)
		batch.Queue(query,
			obs.ProductID.String(),
			obs.CompetitorID,
			obs.Price,
			obs.Currency,
			obs.InStock,
			obs.StockQuantity,
			obs.ObservedAt,
			observedDate,
			obs.Source,
		)
	}

	br := tx.SendBatch(ctx, batch)
	defer br.Close()

	count := 0
	for range observations {
		_, err := br.Exec()
		if err != nil {
			return count, fmt.Errorf("failed to insert price observation: %w", err)
		}
		count++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return count, nil
}

// GetLatestByProduct retrieves the most recent price for each competitor for a product
func (r *PriceObservationRepo) GetLatestByProduct(ctx context.Context, productID uuid.UUID) ([]*database.PriceObservation, error) {
	query := `
		SELECT DISTINCT ON (competitor_id)
			id, product_id, competitor_id, price, currency, in_stock, stock_quantity, observed_at, source
		FROM price_observations
		WHERE product_id = $1
		ORDER BY competitor_id, observed_at DESC
	`

	rows, err := r.client.pool.Query(ctx, query, productID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query latest prices: %w", err)
	}
	defer rows.Close()

	return r.scanPriceObservations(rows)
}

// GetByProductAndCompetitor retrieves price history for a specific product/competitor pair
func (r *PriceObservationRepo) GetByProductAndCompetitor(ctx context.Context, productID uuid.UUID, competitorID int, since time.Time) ([]*database.PriceObservation, error) {
	query := `
		SELECT id, product_id, competitor_id, price, currency, in_stock, stock_quantity, observed_at, source
		FROM price_observations
		WHERE product_id = $1 AND competitor_id = $2 AND observed_at >= $3
		ORDER BY observed_at DESC
	`

	rows, err := r.client.pool.Query(ctx, query, productID.String(), competitorID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query price history: %w", err)
	}
	defer rows.Close()

	return r.scanPriceObservations(rows)
}

// GetPriceHistory retrieves all price observations for a product in the last N days
func (r *PriceObservationRepo) GetPriceHistory(ctx context.Context, productID uuid.UUID, days int) ([]*database.PriceObservation, error) {
	since := time.Now().AddDate(0, 0, -days)
	query := `
		SELECT id, product_id, competitor_id, price, currency, in_stock, stock_quantity, observed_at, source
		FROM price_observations
		WHERE product_id = $1 AND observed_at >= $2
		ORDER BY observed_at DESC
	`

	rows, err := r.client.pool.Query(ctx, query, productID.String(), since)
	if err != nil {
		return nil, fmt.Errorf("failed to query price history: %w", err)
	}
	defer rows.Close()

	return r.scanPriceObservations(rows)
}

func (r *PriceObservationRepo) scanPriceObservations(rows pgx.Rows) ([]*database.PriceObservation, error) {
	var observations []*database.PriceObservation

	for rows.Next() {
		var obs database.PriceObservation
		var productIDStr string

		err := rows.Scan(
			&obs.ID, &productIDStr, &obs.CompetitorID, &obs.Price, &obs.Currency,
			&obs.InStock, &obs.StockQuantity, &obs.ObservedAt, &obs.Source,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan price observation: %w", err)
		}

		obs.ProductID, _ = uuid.Parse(productIDStr)
		observations = append(observations, &obs)
	}

	return observations, rows.Err()
}

// Count returns the total number of price observations
func (r *PriceObservationRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.client.pool.QueryRow(ctx, "SELECT COUNT(*) FROM price_observations").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count price observations: %w", err)
	}
	return count, nil
}

// DeleteOlderThan removes price observations older than the specified time
func (r *PriceObservationRepo) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.client.pool.Exec(ctx, "DELETE FROM price_observations WHERE observed_at < $1", before)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old observations: %w", err)
	}
	return result.RowsAffected(), nil
}
