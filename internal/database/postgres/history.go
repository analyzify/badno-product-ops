package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/badno/badops/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// HistoryRepo implements the HistoryRepository interface for PostgreSQL
type HistoryRepo struct {
	client *Client
}

// NewHistoryRepo creates a new PostgreSQL history repository
func NewHistoryRepo(client *Client) *HistoryRepo {
	return &HistoryRepo{client: client}
}

// Add inserts a new operation history entry
func (r *HistoryRepo) Add(ctx context.Context, entry *database.OperationHistory) error {
	query := `
		INSERT INTO operation_history (action, source, count, details, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	if entry.StartedAt.IsZero() {
		entry.StartedAt = time.Now()
	}

	err := r.client.pool.QueryRow(ctx, query,
		entry.Action,
		entry.Source,
		entry.Count,
		entry.Details,
		entry.StartedAt,
		entry.CompletedAt,
	).Scan(&entry.ID)

	if err != nil {
		return fmt.Errorf("failed to add history entry: %w", err)
	}

	return nil
}

// GetRecent retrieves the most recent history entries
func (r *HistoryRepo) GetRecent(ctx context.Context, limit int) ([]*database.OperationHistory, error) {
	query := `
		SELECT id, action, source, count, details, started_at, completed_at
		FROM operation_history
		ORDER BY started_at DESC
		LIMIT $1
	`

	rows, err := r.client.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	return r.scanHistory(rows)
}

// GetByAction retrieves history entries for a specific action
func (r *HistoryRepo) GetByAction(ctx context.Context, action string, limit int) ([]*database.OperationHistory, error) {
	query := `
		SELECT id, action, source, count, details, started_at, completed_at
		FROM operation_history
		WHERE action = $1
		ORDER BY started_at DESC
		LIMIT $2
	`

	rows, err := r.client.pool.Query(ctx, query, action, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query history by action: %w", err)
	}
	defer rows.Close()

	return r.scanHistory(rows)
}

func (r *HistoryRepo) scanHistory(rows pgx.Rows) ([]*database.OperationHistory, error) {
	var entries []*database.OperationHistory

	for rows.Next() {
		var entry database.OperationHistory
		err := rows.Scan(
			&entry.ID, &entry.Action, &entry.Source, &entry.Count,
			&entry.Details, &entry.StartedAt, &entry.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan history entry: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// MarkCompleted marks an operation as completed
func (r *HistoryRepo) MarkCompleted(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := r.client.pool.Exec(ctx,
		"UPDATE operation_history SET completed_at = $1 WHERE id = $2",
		now, id,
	)
	if err != nil {
		return fmt.Errorf("failed to mark history completed: %w", err)
	}
	return nil
}

// EnhancementLogRepo implements enhancement logging
type EnhancementLogRepo struct {
	client *Client
}

// NewEnhancementLogRepo creates a new PostgreSQL enhancement log repository
func NewEnhancementLogRepo(client *Client) *EnhancementLogRepo {
	return &EnhancementLogRepo{client: client}
}

// Add inserts a new enhancement log entry
func (r *EnhancementLogRepo) Add(ctx context.Context, entry *database.EnhancementLog) error {
	query := `
		INSERT INTO enhancement_log (product_id, source, action, fields_added, success, error)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	err := r.client.pool.QueryRow(ctx, query,
		entry.ProductID.String(),
		entry.Source,
		entry.Action,
		entry.FieldsAdded,
		entry.Success,
		entry.Error,
	).Scan(&entry.ID, &entry.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to add enhancement log: %w", err)
	}

	return nil
}

// GetByProduct retrieves enhancement logs for a product
func (r *EnhancementLogRepo) GetByProduct(ctx context.Context, productID uuid.UUID) ([]*database.EnhancementLog, error) {
	query := `
		SELECT id, product_id, source, action, fields_added, success, error, created_at
		FROM enhancement_log
		WHERE product_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.client.pool.Query(ctx, query, productID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query enhancement logs: %w", err)
	}
	defer rows.Close()

	var entries []*database.EnhancementLog
	for rows.Next() {
		var entry database.EnhancementLog
		var productIDStr string

		err := rows.Scan(
			&entry.ID, &productIDStr, &entry.Source, &entry.Action,
			&entry.FieldsAdded, &entry.Success, &entry.Error, &entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan enhancement log: %w", err)
		}

		entry.ProductID, _ = uuid.Parse(productIDStr)
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// GetBySource retrieves enhancement logs by source
func (r *EnhancementLogRepo) GetBySource(ctx context.Context, source string, limit int) ([]*database.EnhancementLog, error) {
	query := `
		SELECT id, product_id, source, action, fields_added, success, error, created_at
		FROM enhancement_log
		WHERE source = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.client.pool.Query(ctx, query, source, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query enhancement logs by source: %w", err)
	}
	defer rows.Close()

	var entries []*database.EnhancementLog
	for rows.Next() {
		var entry database.EnhancementLog
		var productIDStr string

		err := rows.Scan(
			&entry.ID, &productIDStr, &entry.Source, &entry.Action,
			&entry.FieldsAdded, &entry.Success, &entry.Error, &entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan enhancement log: %w", err)
		}

		entry.ProductID, _ = uuid.Parse(productIDStr)
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// ImageRepo implements the ImageRepository interface for PostgreSQL
type ImageRepo struct {
	client *Client
}

// NewImageRepo creates a new PostgreSQL image repository
func NewImageRepo(client *Client) *ImageRepo {
	return &ImageRepo{client: client}
}

// Create inserts a new product image
func (r *ImageRepo) Create(ctx context.Context, image *database.ProductImage) error {
	if image.ID == uuid.Nil {
		image.ID = uuid.New()
	}

	query := `
		INSERT INTO product_images (
			id, product_id, source_url, source, local_path,
			width, height, position, alt_text, status, resized_paths, downloaded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	resizedJSON := []byte("{}")
	if image.ResizedPaths != nil {
		resizedJSON, _ = json.Marshal(image.ResizedPaths)
	}

	_, err := r.client.pool.Exec(ctx, query,
		image.ID.String(),
		image.ProductID.String(),
		image.SourceURL,
		image.Source,
		image.LocalPath,
		image.Width,
		image.Height,
		image.Position,
		image.AltText,
		image.Status,
		resizedJSON,
		image.DownloadedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create product image: %w", err)
	}

	return nil
}

// GetByProduct retrieves all images for a product
func (r *ImageRepo) GetByProduct(ctx context.Context, productID uuid.UUID) ([]*database.ProductImage, error) {
	query := `
		SELECT id, product_id, source_url, source, local_path,
		       width, height, position, alt_text, status, resized_paths, downloaded_at, created_at
		FROM product_images
		WHERE product_id = $1
		ORDER BY position
	`

	rows, err := r.client.pool.Query(ctx, query, productID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query images: %w", err)
	}
	defer rows.Close()

	var images []*database.ProductImage
	for rows.Next() {
		var img database.ProductImage
		var idStr, productIDStr string
		var resizedJSON []byte

		err := rows.Scan(
			&idStr, &productIDStr, &img.SourceURL, &img.Source, &img.LocalPath,
			&img.Width, &img.Height, &img.Position, &img.AltText, &img.Status,
			&resizedJSON, &img.DownloadedAt, &img.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan image: %w", err)
		}

		img.ID, _ = uuid.Parse(idStr)
		img.ProductID, _ = uuid.Parse(productIDStr)

		if len(resizedJSON) > 0 {
			json.Unmarshal(resizedJSON, &img.ResizedPaths)
		}

		images = append(images, &img)
	}

	return images, rows.Err()
}

// Update updates an existing image
func (r *ImageRepo) Update(ctx context.Context, image *database.ProductImage) error {
	query := `
		UPDATE product_images SET
			local_path = $2, width = $3, height = $4, position = $5,
			alt_text = $6, status = $7, resized_paths = $8, downloaded_at = $9
		WHERE id = $1
	`

	resizedJSON := []byte("{}")
	if image.ResizedPaths != nil {
		resizedJSON, _ = json.Marshal(image.ResizedPaths)
	}

	_, err := r.client.pool.Exec(ctx, query,
		image.ID.String(),
		image.LocalPath,
		image.Width,
		image.Height,
		image.Position,
		image.AltText,
		image.Status,
		resizedJSON,
		image.DownloadedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update image: %w", err)
	}

	return nil
}

// Delete removes an image
func (r *ImageRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.client.pool.Exec(ctx, "DELETE FROM product_images WHERE id = $1", id.String())
	if err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}
	return nil
}

// BulkUpsert inserts or updates multiple images
func (r *ImageRepo) BulkUpsert(ctx context.Context, images []*database.ProductImage) (int, error) {
	if len(images) == 0 {
		return 0, nil
	}

	tx, err := r.client.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO product_images (
			id, product_id, source_url, source, local_path,
			width, height, position, alt_text, status, resized_paths, downloaded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			local_path = EXCLUDED.local_path,
			width = EXCLUDED.width,
			height = EXCLUDED.height,
			position = EXCLUDED.position,
			alt_text = EXCLUDED.alt_text,
			status = EXCLUDED.status,
			resized_paths = EXCLUDED.resized_paths,
			downloaded_at = EXCLUDED.downloaded_at
	`

	batch := &pgx.Batch{}
	for _, img := range images {
		if img.ID == uuid.Nil {
			img.ID = uuid.New()
		}

		resizedJSON := []byte("{}")
		if img.ResizedPaths != nil {
			resizedJSON, _ = json.Marshal(img.ResizedPaths)
		}

		batch.Queue(query,
			img.ID.String(),
			img.ProductID.String(),
			img.SourceURL,
			img.Source,
			img.LocalPath,
			img.Width,
			img.Height,
			img.Position,
			img.AltText,
			img.Status,
			resizedJSON,
			img.DownloadedAt,
		)
	}

	br := tx.SendBatch(ctx, batch)
	defer br.Close()

	count := 0
	for range images {
		_, err := br.Exec()
		if err != nil {
			return count, fmt.Errorf("failed to upsert image: %w", err)
		}
		count++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return count, nil
}

// PropertyRepo implements the PropertyRepository interface
type PropertyRepo struct {
	client *Client
}

// NewPropertyRepo creates a new PostgreSQL property repository
func NewPropertyRepo(client *Client) *PropertyRepo {
	return &PropertyRepo{client: client}
}

// Create inserts a new product property
func (r *PropertyRepo) Create(ctx context.Context, property *database.ProductProperty) error {
	query := `
		INSERT INTO product_properties (product_id, code, name, value, unit, source)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (product_id, code, source) DO UPDATE SET
			name = EXCLUDED.name,
			value = EXCLUDED.value,
			unit = EXCLUDED.unit
	`

	_, err := r.client.pool.Exec(ctx, query,
		property.ProductID.String(),
		property.Code,
		property.Name,
		property.Value,
		property.Unit,
		property.Source,
	)

	if err != nil {
		return fmt.Errorf("failed to create property: %w", err)
	}

	return nil
}

// GetByProduct retrieves all properties for a product
func (r *PropertyRepo) GetByProduct(ctx context.Context, productID uuid.UUID) ([]*database.ProductProperty, error) {
	query := `
		SELECT product_id, code, name, value, unit, source
		FROM product_properties
		WHERE product_id = $1
		ORDER BY source, name
	`

	rows, err := r.client.pool.Query(ctx, query, productID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query properties: %w", err)
	}
	defer rows.Close()

	var properties []*database.ProductProperty
	for rows.Next() {
		var prop database.ProductProperty
		var productIDStr string

		err := rows.Scan(&productIDStr, &prop.Code, &prop.Name, &prop.Value, &prop.Unit, &prop.Source)
		if err != nil {
			return nil, fmt.Errorf("failed to scan property: %w", err)
		}

		prop.ProductID, _ = uuid.Parse(productIDStr)
		properties = append(properties, &prop)
	}

	return properties, rows.Err()
}

// BulkUpsert inserts or updates multiple properties
func (r *PropertyRepo) BulkUpsert(ctx context.Context, properties []*database.ProductProperty) (int, error) {
	if len(properties) == 0 {
		return 0, nil
	}

	tx, err := r.client.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO product_properties (product_id, code, name, value, unit, source)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (product_id, code, source) DO UPDATE SET
			name = EXCLUDED.name,
			value = EXCLUDED.value,
			unit = EXCLUDED.unit
	`

	batch := &pgx.Batch{}
	for _, prop := range properties {
		batch.Queue(query,
			prop.ProductID.String(),
			prop.Code,
			prop.Name,
			prop.Value,
			prop.Unit,
			prop.Source,
		)
	}

	br := tx.SendBatch(ctx, batch)
	defer br.Close()

	count := 0
	for range properties {
		_, err := br.Exec()
		if err != nil {
			return count, fmt.Errorf("failed to upsert property: %w", err)
		}
		count++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return count, nil
}

// DeleteByProduct removes all properties for a product
func (r *PropertyRepo) DeleteByProduct(ctx context.Context, productID uuid.UUID) error {
	_, err := r.client.pool.Exec(ctx, "DELETE FROM product_properties WHERE product_id = $1", productID.String())
	if err != nil {
		return fmt.Errorf("failed to delete properties: %w", err)
	}
	return nil
}
