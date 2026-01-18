package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/badno/badops/internal/database"
	"github.com/badno/badops/pkg/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ProductRepo implements the ProductRepository interface for PostgreSQL
type ProductRepo struct {
	client *Client
}

// NewProductRepo creates a new PostgreSQL product repository
func NewProductRepo(client *Client) *ProductRepo {
	return &ProductRepo{client: client}
}

// Create inserts a new product into the database
func (r *ProductRepo) Create(ctx context.Context, product *models.EnhancedProduct) error {
	if product.ID == "" {
		product.ID = uuid.New().String()
	}

	query := `
		INSERT INTO products (
			id, sku, handle, barcode, nobb_number,
			title, description, vendor, product_type, tags,
			price, cost, compare_at_price, currency, profit_margin,
			weight_value, weight_unit, length_mm, width_mm, height_mm,
			status, specifications, created_at, updated_at,
			legacy_matched_url, legacy_match_score
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20,
			$21, $22, $23, $24,
			$25, $26
		)
	`

	var price, cost, compareAt *float64
	var currency string
	if product.Price != nil {
		price = &product.Price.Amount
		cost = &product.Price.CostPerItem
		compareAt = &product.Price.CompareAt
		currency = product.Price.Currency
	}
	if currency == "" {
		currency = "NOK"
	}

	var weightValue *float64
	var weightUnit string
	if product.Weight != nil {
		weightValue = &product.Weight.Value
		weightUnit = product.Weight.Unit
	}

	var length, width, height *float64
	if product.Dimensions != nil {
		length = &product.Dimensions.Length
		width = &product.Dimensions.Width
		height = &product.Dimensions.Height
	}

	specsJSON, _ := json.Marshal(product.Specifications)

	now := time.Now()
	if product.CreatedAt.IsZero() {
		product.CreatedAt = now
	}
	product.UpdatedAt = now

	_, err := r.client.pool.Exec(ctx, query,
		product.ID, product.SKU, product.Handle, product.Barcode, product.NOBBNumber,
		product.Title, product.Description, product.Vendor, product.ProductType, product.Tags,
		price, cost, compareAt, currency, nil,
		weightValue, weightUnit, length, width, height,
		string(product.Status), specsJSON, product.CreatedAt, product.UpdatedAt,
		product.LegacyMatchedURL, product.LegacyMatchScore,
	)

	if err != nil {
		return fmt.Errorf("failed to create product: %w", err)
	}

	return nil
}

// GetByID retrieves a product by its UUID
func (r *ProductRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EnhancedProduct, error) {
	return r.getByField(ctx, "id", id.String())
}

// GetBySKU retrieves a product by its SKU
func (r *ProductRepo) GetBySKU(ctx context.Context, sku string) (*models.EnhancedProduct, error) {
	return r.getByField(ctx, "sku", sku)
}

// GetByBarcode retrieves a product by its barcode
func (r *ProductRepo) GetByBarcode(ctx context.Context, barcode string) (*models.EnhancedProduct, error) {
	return r.getByField(ctx, "barcode", barcode)
}

func (r *ProductRepo) getByField(ctx context.Context, field, value string) (*models.EnhancedProduct, error) {
	query := fmt.Sprintf(`
		SELECT
			id, sku, handle, barcode, nobb_number,
			title, description, vendor, product_type, tags,
			price, cost, compare_at_price, currency,
			weight_value, weight_unit, length_mm, width_mm, height_mm,
			status, specifications, created_at, updated_at,
			legacy_matched_url, legacy_match_score
		FROM products
		WHERE %s = $1
	`, field)

	row := r.client.pool.QueryRow(ctx, query, value)
	return r.scanProduct(row)
}

func (r *ProductRepo) scanProduct(row pgx.Row) (*models.EnhancedProduct, error) {
	var p models.EnhancedProduct
	var price, cost, compareAt *float64
	var currency *string
	var weightValue *float64
	var weightUnit *string
	var length, width, height *float64
	var specs []byte
	var status string

	err := row.Scan(
		&p.ID, &p.SKU, &p.Handle, &p.Barcode, &p.NOBBNumber,
		&p.Title, &p.Description, &p.Vendor, &p.ProductType, &p.Tags,
		&price, &cost, &compareAt, &currency,
		&weightValue, &weightUnit, &length, &width, &height,
		&status, &specs, &p.CreatedAt, &p.UpdatedAt,
		&p.LegacyMatchedURL, &p.LegacyMatchScore,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan product: %w", err)
	}

	p.Status = models.ProductStatus(status)

	if price != nil || cost != nil || compareAt != nil {
		p.Price = &models.Price{}
		if price != nil {
			p.Price.Amount = *price
		}
		if cost != nil {
			p.Price.CostPerItem = *cost
		}
		if compareAt != nil {
			p.Price.CompareAt = *compareAt
		}
		if currency != nil {
			p.Price.Currency = *currency
		}
	}

	if weightValue != nil {
		p.Weight = &models.Weight{Value: *weightValue}
		if weightUnit != nil {
			p.Weight.Unit = *weightUnit
		}
	}

	if length != nil || width != nil || height != nil {
		p.Dimensions = &models.Dimensions{Unit: "mm"}
		if length != nil {
			p.Dimensions.Length = *length
		}
		if width != nil {
			p.Dimensions.Width = *width
		}
		if height != nil {
			p.Dimensions.Height = *height
		}
	}

	if len(specs) > 0 {
		json.Unmarshal(specs, &p.Specifications)
	}

	return &p, nil
}

// Update updates an existing product
func (r *ProductRepo) Update(ctx context.Context, product *models.EnhancedProduct) error {
	query := `
		UPDATE products SET
			handle = $2, barcode = $3, nobb_number = $4,
			title = $5, description = $6, vendor = $7, product_type = $8, tags = $9,
			price = $10, cost = $11, compare_at_price = $12, currency = $13,
			weight_value = $14, weight_unit = $15,
			length_mm = $16, width_mm = $17, height_mm = $18,
			status = $19, specifications = $20,
			legacy_matched_url = $21, legacy_match_score = $22
		WHERE id = $1
	`

	var price, cost, compareAt *float64
	var currency string
	if product.Price != nil {
		price = &product.Price.Amount
		cost = &product.Price.CostPerItem
		compareAt = &product.Price.CompareAt
		currency = product.Price.Currency
	}
	if currency == "" {
		currency = "NOK"
	}

	var weightValue *float64
	var weightUnit string
	if product.Weight != nil {
		weightValue = &product.Weight.Value
		weightUnit = product.Weight.Unit
	}

	var length, width, height *float64
	if product.Dimensions != nil {
		length = &product.Dimensions.Length
		width = &product.Dimensions.Width
		height = &product.Dimensions.Height
	}

	specsJSON, _ := json.Marshal(product.Specifications)

	_, err := r.client.pool.Exec(ctx, query,
		product.ID, product.Handle, product.Barcode, product.NOBBNumber,
		product.Title, product.Description, product.Vendor, product.ProductType, product.Tags,
		price, cost, compareAt, currency,
		weightValue, weightUnit, length, width, height,
		string(product.Status), specsJSON,
		product.LegacyMatchedURL, product.LegacyMatchScore,
	)

	if err != nil {
		return fmt.Errorf("failed to update product: %w", err)
	}

	return nil
}

// Delete removes a product from the database
func (r *ProductRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.client.pool.Exec(ctx, "DELETE FROM products WHERE id = $1", id.String())
	if err != nil {
		return fmt.Errorf("failed to delete product: %w", err)
	}
	return nil
}

// BulkUpsert inserts or updates multiple products
func (r *ProductRepo) BulkUpsert(ctx context.Context, products []*models.EnhancedProduct) (int, error) {
	if len(products) == 0 {
		return 0, nil
	}

	tx, err := r.client.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO products (
			id, sku, handle, barcode, nobb_number,
			title, description, vendor, product_type, tags,
			price, cost, compare_at_price, currency,
			weight_value, weight_unit, length_mm, width_mm, height_mm,
			status, specifications, created_at, updated_at,
			legacy_matched_url, legacy_match_score
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14,
			$15, $16, $17, $18, $19,
			$20, $21, $22, $23,
			$24, $25
		)
		ON CONFLICT (sku) DO UPDATE SET
			handle = EXCLUDED.handle,
			barcode = COALESCE(NULLIF(EXCLUDED.barcode, ''), products.barcode),
			nobb_number = COALESCE(NULLIF(EXCLUDED.nobb_number, ''), products.nobb_number),
			title = EXCLUDED.title,
			description = COALESCE(NULLIF(EXCLUDED.description, ''), products.description),
			vendor = EXCLUDED.vendor,
			product_type = EXCLUDED.product_type,
			tags = EXCLUDED.tags,
			price = COALESCE(EXCLUDED.price, products.price),
			cost = COALESCE(EXCLUDED.cost, products.cost),
			compare_at_price = COALESCE(EXCLUDED.compare_at_price, products.compare_at_price),
			currency = EXCLUDED.currency,
			weight_value = COALESCE(EXCLUDED.weight_value, products.weight_value),
			weight_unit = COALESCE(NULLIF(EXCLUDED.weight_unit, ''), products.weight_unit),
			length_mm = COALESCE(EXCLUDED.length_mm, products.length_mm),
			width_mm = COALESCE(EXCLUDED.width_mm, products.width_mm),
			height_mm = COALESCE(EXCLUDED.height_mm, products.height_mm),
			status = EXCLUDED.status,
			specifications = products.specifications || EXCLUDED.specifications,
			updated_at = NOW(),
			legacy_matched_url = COALESCE(NULLIF(EXCLUDED.legacy_matched_url, ''), products.legacy_matched_url),
			legacy_match_score = COALESCE(EXCLUDED.legacy_match_score, products.legacy_match_score)
	`

	batch := &pgx.Batch{}
	now := time.Now()

	for _, p := range products {
		if p.ID == "" {
			p.ID = uuid.New().String()
		}

		var price, cost, compareAt *float64
		var currency string = "NOK"
		if p.Price != nil {
			price = &p.Price.Amount
			cost = &p.Price.CostPerItem
			compareAt = &p.Price.CompareAt
			currency = p.Price.Currency
		}

		var weightValue *float64
		var weightUnit string
		if p.Weight != nil {
			weightValue = &p.Weight.Value
			weightUnit = p.Weight.Unit
		}

		var length, width, height *float64
		if p.Dimensions != nil {
			length = &p.Dimensions.Length
			width = &p.Dimensions.Width
			height = &p.Dimensions.Height
		}

		specsJSON, _ := json.Marshal(p.Specifications)

		createdAt := p.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}

		batch.Queue(query,
			p.ID, p.SKU, p.Handle, p.Barcode, p.NOBBNumber,
			p.Title, p.Description, p.Vendor, p.ProductType, p.Tags,
			price, cost, compareAt, currency,
			weightValue, weightUnit, length, width, height,
			string(p.Status), specsJSON, createdAt, now,
			p.LegacyMatchedURL, p.LegacyMatchScore,
		)
	}

	br := tx.SendBatch(ctx, batch)
	defer br.Close()

	count := 0
	for range products {
		_, err := br.Exec()
		if err != nil {
			return count, fmt.Errorf("failed to upsert product: %w", err)
		}
		count++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return count, nil
}

// GetAll retrieves products with optional filtering
func (r *ProductRepo) GetAll(ctx context.Context, opts database.QueryOptions) ([]*models.EnhancedProduct, error) {
	var conditions []string
	var args []interface{}
	argNum := 1

	if opts.Vendor != "" {
		conditions = append(conditions, fmt.Sprintf("vendor = $%d", argNum))
		args = append(args, opts.Vendor)
		argNum++
	}

	if opts.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, string(opts.Status))
		argNum++
	}

	query := `
		SELECT
			id, sku, handle, barcode, nobb_number,
			title, description, vendor, product_type, tags,
			price, cost, compare_at_price, currency,
			weight_value, weight_unit, length_mm, width_mm, height_mm,
			status, specifications, created_at, updated_at,
			legacy_matched_url, legacy_match_score
		FROM products
	`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	orderBy := opts.OrderBy
	if orderBy == "" {
		orderBy = "updated_at"
	}
	orderDir := opts.OrderDir
	if orderDir == "" {
		orderDir = "DESC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", orderBy, orderDir)

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	rows, err := r.client.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	return r.scanProducts(rows)
}

func (r *ProductRepo) scanProducts(rows pgx.Rows) ([]*models.EnhancedProduct, error) {
	var products []*models.EnhancedProduct

	for rows.Next() {
		var p models.EnhancedProduct
		var price, cost, compareAt *float64
		var currency *string
		var weightValue *float64
		var weightUnit *string
		var length, width, height *float64
		var specs []byte
		var status string

		err := rows.Scan(
			&p.ID, &p.SKU, &p.Handle, &p.Barcode, &p.NOBBNumber,
			&p.Title, &p.Description, &p.Vendor, &p.ProductType, &p.Tags,
			&price, &cost, &compareAt, &currency,
			&weightValue, &weightUnit, &length, &width, &height,
			&status, &specs, &p.CreatedAt, &p.UpdatedAt,
			&p.LegacyMatchedURL, &p.LegacyMatchScore,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}

		p.Status = models.ProductStatus(status)

		if price != nil || cost != nil || compareAt != nil {
			p.Price = &models.Price{}
			if price != nil {
				p.Price.Amount = *price
			}
			if cost != nil {
				p.Price.CostPerItem = *cost
			}
			if compareAt != nil {
				p.Price.CompareAt = *compareAt
			}
			if currency != nil {
				p.Price.Currency = *currency
			}
		}

		if weightValue != nil {
			p.Weight = &models.Weight{Value: *weightValue}
			if weightUnit != nil {
				p.Weight.Unit = *weightUnit
			}
		}

		if length != nil || width != nil || height != nil {
			p.Dimensions = &models.Dimensions{Unit: "mm"}
			if length != nil {
				p.Dimensions.Length = *length
			}
			if width != nil {
				p.Dimensions.Width = *width
			}
			if height != nil {
				p.Dimensions.Height = *height
			}
		}

		if len(specs) > 0 {
			json.Unmarshal(specs, &p.Specifications)
		}

		products = append(products, &p)
	}

	return products, rows.Err()
}

// GetByVendor retrieves all products for a specific vendor
func (r *ProductRepo) GetByVendor(ctx context.Context, vendor string) ([]*models.EnhancedProduct, error) {
	return r.GetAll(ctx, database.QueryOptions{Vendor: vendor})
}

// GetByStatus retrieves all products with a specific status
func (r *ProductRepo) GetByStatus(ctx context.Context, status models.ProductStatus) ([]*models.EnhancedProduct, error) {
	return r.GetAll(ctx, database.QueryOptions{Status: status})
}

// Count returns the total number of products
func (r *ProductRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.client.pool.QueryRow(ctx, "SELECT COUNT(*) FROM products").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count products: %w", err)
	}
	return count, nil
}

// CountByVendor returns product counts grouped by vendor
func (r *ProductRepo) CountByVendor(ctx context.Context) (map[string]int64, error) {
	query := `SELECT vendor, COUNT(*) FROM products GROUP BY vendor ORDER BY COUNT(*) DESC`
	rows, err := r.client.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count by vendor: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var vendor string
		var count int64
		if err := rows.Scan(&vendor, &count); err != nil {
			return nil, err
		}
		counts[vendor] = count
	}
	return counts, rows.Err()
}

// CountByStatus returns product counts grouped by status
func (r *ProductRepo) CountByStatus(ctx context.Context) (map[models.ProductStatus]int64, error) {
	query := `SELECT status, COUNT(*) FROM products GROUP BY status`
	rows, err := r.client.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[models.ProductStatus]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[models.ProductStatus(status)] = count
	}
	return counts, rows.Err()
}
