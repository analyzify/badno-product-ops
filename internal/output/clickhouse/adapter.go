package clickhouse

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/badno/badops/internal/output"
	"github.com/badno/badops/pkg/models"
)

const AdapterName = "clickhouse"

// Config holds ClickHouse connection configuration
type Config struct {
	Host        string // ClickHouse host
	Port        int    // ClickHouse port (default: 9000)
	Database    string // Database name
	Username    string // Username
	Password    string // Password
	UsernameEnv string // Environment variable for username
	PasswordEnv string // Environment variable for password
	Table       string // Target table name
	Secure      bool   // Use TLS
}

// Adapter implements the output.Adapter interface for ClickHouse
type Adapter struct {
	*output.BaseAdapter
	config Config
	db     *sql.DB
}

// NewAdapter creates a new ClickHouse output adapter
func NewAdapter(cfg Config) *Adapter {
	if cfg.Port == 0 {
		cfg.Port = 9000
	}
	if cfg.Database == "" {
		cfg.Database = "default"
	}
	if cfg.Table == "" {
		cfg.Table = "products"
	}

	return &Adapter{
		BaseAdapter: output.NewBaseAdapter(
			AdapterName,
			[]output.Format{}, // ClickHouse uses its own format
		),
		config: cfg,
	}
}

// SupportsFormat - ClickHouse adapter doesn't use file formats
func (a *Adapter) SupportsFormat(format output.Format) bool {
	return false
}

// Connect establishes connection to ClickHouse
func (a *Adapter) Connect(ctx context.Context) error {
	// Resolve credentials from environment
	username := a.config.Username
	if username == "" && a.config.UsernameEnv != "" {
		username = os.Getenv(a.config.UsernameEnv)
	}
	password := a.config.Password
	if password == "" && a.config.PasswordEnv != "" {
		password = os.Getenv(a.config.PasswordEnv)
	}

	// Build connection string
	protocol := "tcp"
	if a.config.Secure {
		protocol = "tls"
	}

	dsn := fmt.Sprintf("%s://%s:%d?database=%s",
		protocol, a.config.Host, a.config.Port, a.config.Database)

	if username != "" {
		dsn += fmt.Sprintf("&username=%s", username)
	}
	if password != "" {
		dsn += fmt.Sprintf("&password=%s", password)
	}

	// Open connection
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return fmt.Errorf("failed to open ClickHouse connection: %w", err)
	}

	a.db = db
	return a.Test(ctx)
}

// Close cleans up resources
func (a *Adapter) Close() error {
	if a.db != nil {
		a.db.Close()
	}
	a.SetConnected(false)
	return nil
}

// Test verifies connectivity to ClickHouse
func (a *Adapter) Test(ctx context.Context) error {
	if a.db == nil {
		return fmt.Errorf("not connected to ClickHouse")
	}

	if err := a.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ClickHouse ping failed: %w", err)
	}

	a.SetConnected(true)
	return nil
}

// ExportProducts exports products to ClickHouse
func (a *Adapter) ExportProducts(ctx context.Context, products []models.EnhancedProduct, opts output.ExportOptions) (*output.ExportResult, error) {
	result := &output.ExportResult{
		StartedAt: time.Now(),
	}

	if !a.IsConnected() {
		if err := a.Connect(ctx); err != nil {
			result.Error = err
			return result, err
		}
	}

	// Filter products
	filteredProducts := products
	if opts.OnlyEnhanced {
		filteredProducts = make([]models.EnhancedProduct, 0)
		for _, p := range products {
			if len(p.Enhancements) > 0 {
				filteredProducts = append(filteredProducts, p)
			}
		}
	}
	if len(opts.SKUs) > 0 {
		skuSet := make(map[string]bool)
		for _, sku := range opts.SKUs {
			skuSet[sku] = true
		}
		temp := make([]models.EnhancedProduct, 0)
		for _, p := range filteredProducts {
			if skuSet[p.SKU] {
				temp = append(temp, p)
			}
		}
		filteredProducts = temp
	}

	if opts.DryRun {
		result.ProductsExported = len(filteredProducts)
		result.Success = true
		result.Details = fmt.Sprintf("Dry run: would insert %d products into ClickHouse", len(filteredProducts))
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Ensure table exists
	if err := a.ensureTable(ctx); err != nil {
		result.Error = err
		return result, err
	}

	// Insert products
	inserted, imagesExported, err := a.insertProducts(ctx, filteredProducts)
	if err != nil {
		result.Error = err
	}

	result.Destination = fmt.Sprintf("%s:%d/%s.%s", a.config.Host, a.config.Port, a.config.Database, a.config.Table)
	result.ProductsExported = inserted
	result.ImagesExported = imagesExported
	result.Success = err == nil
	result.Details = fmt.Sprintf("Inserted %d/%d products into ClickHouse", inserted, len(filteredProducts))
	result.CompletedAt = time.Now()

	return result, nil
}

// ensureTable creates the products table if it doesn't exist
func (a *Adapter) ensureTable(ctx context.Context) error {
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			sku String,
			handle String,
			barcode String,
			nobb_number String,
			title String,
			description String,
			vendor String,
			product_type String,
			tags Array(String),
			price_amount Float64,
			price_currency String,
			weight_value Float64,
			weight_unit String,
			dimensions_length Float64,
			dimensions_width Float64,
			dimensions_height Float64,
			dimensions_unit String,
			images Array(String),
			specifications String,
			properties String,
			suppliers String,
			enhancements String,
			status String,
			created_at DateTime,
			updated_at DateTime,
			exported_at DateTime DEFAULT now()
		) ENGINE = MergeTree()
		ORDER BY (sku, exported_at)
	`, a.config.Table)

	_, err := a.db.ExecContext(ctx, createSQL)
	return err
}

// insertProducts inserts products into ClickHouse
func (a *Adapter) insertProducts(ctx context.Context, products []models.EnhancedProduct) (int, int, error) {
	if len(products) == 0 {
		return 0, 0, nil
	}

	// Build batch insert
	columns := []string{
		"sku", "handle", "barcode", "nobb_number",
		"title", "description", "vendor", "product_type", "tags",
		"price_amount", "price_currency",
		"weight_value", "weight_unit",
		"dimensions_length", "dimensions_width", "dimensions_height", "dimensions_unit",
		"images", "specifications", "properties", "suppliers", "enhancements",
		"status", "created_at", "updated_at",
	}

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		a.config.Table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}

	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		tx.Rollback()
		return 0, 0, err
	}
	defer stmt.Close()

	inserted := 0
	imagesExported := 0

	for _, p := range products {
		// Extract values
		priceAmount := float64(0)
		priceCurrency := ""
		if p.Price != nil {
			priceAmount = p.Price.Amount
			priceCurrency = p.Price.Currency
		}

		weightValue := float64(0)
		weightUnit := ""
		if p.Weight != nil {
			weightValue = p.Weight.Value
			weightUnit = p.Weight.Unit
		}

		dimLength := float64(0)
		dimWidth := float64(0)
		dimHeight := float64(0)
		dimUnit := ""
		if p.Dimensions != nil {
			dimLength = p.Dimensions.Length
			dimWidth = p.Dimensions.Width
			dimHeight = p.Dimensions.Height
			dimUnit = p.Dimensions.Unit
		}

		// Collect image URLs
		var imageURLs []string
		for _, img := range p.Images {
			imageURLs = append(imageURLs, img.SourceURL)
			imagesExported++
		}

		// Serialize complex fields to JSON
		specsJSON, _ := json.Marshal(p.Specifications)
		propsJSON, _ := json.Marshal(p.Properties)
		suppliersJSON, _ := json.Marshal(p.Suppliers)
		enhancementsJSON, _ := json.Marshal(p.Enhancements)

		_, err := stmt.ExecContext(ctx,
			p.SKU, p.Handle, p.Barcode, p.NOBBNumber,
			p.Title, p.Description, p.Vendor, p.ProductType, p.Tags,
			priceAmount, priceCurrency,
			weightValue, weightUnit,
			dimLength, dimWidth, dimHeight, dimUnit,
			imageURLs, string(specsJSON), string(propsJSON), string(suppliersJSON), string(enhancementsJSON),
			string(p.Status), p.CreatedAt, p.UpdatedAt,
		)

		if err != nil {
			// Log error but continue with other products
			continue
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return inserted, imagesExported, err
	}

	return inserted, imagesExported, nil
}
