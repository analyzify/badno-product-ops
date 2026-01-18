package database

import (
	"context"
	"time"

	"github.com/badno/badops/pkg/models"
	"github.com/google/uuid"
)

// ProductRepository defines the interface for product data access
type ProductRepository interface {
	// CRUD operations
	Create(ctx context.Context, product *models.EnhancedProduct) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.EnhancedProduct, error)
	GetBySKU(ctx context.Context, sku string) (*models.EnhancedProduct, error)
	GetByBarcode(ctx context.Context, barcode string) (*models.EnhancedProduct, error)
	Update(ctx context.Context, product *models.EnhancedProduct) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Bulk operations
	BulkUpsert(ctx context.Context, products []*models.EnhancedProduct) (int, error)
	GetAll(ctx context.Context, opts QueryOptions) ([]*models.EnhancedProduct, error)
	GetByVendor(ctx context.Context, vendor string) ([]*models.EnhancedProduct, error)
	GetByStatus(ctx context.Context, status models.ProductStatus) ([]*models.EnhancedProduct, error)

	// Counts and stats
	Count(ctx context.Context) (int64, error)
	CountByVendor(ctx context.Context) (map[string]int64, error)
	CountByStatus(ctx context.Context) (map[models.ProductStatus]int64, error)
}

// CompetitorRepository defines the interface for competitor data access
type CompetitorRepository interface {
	Create(ctx context.Context, competitor *Competitor) error
	GetByID(ctx context.Context, id int) (*Competitor, error)
	GetByName(ctx context.Context, name string) (*Competitor, error)
	GetAll(ctx context.Context) ([]*Competitor, error)
	Update(ctx context.Context, competitor *Competitor) error
	Delete(ctx context.Context, id int) error
	Count(ctx context.Context) (int64, error)
}

// CompetitorProductRepository defines the interface for competitor product links
type CompetitorProductRepository interface {
	Create(ctx context.Context, link *CompetitorProduct) error
	GetByProductAndCompetitor(ctx context.Context, productID uuid.UUID, competitorID int) (*CompetitorProduct, error)
	GetByProduct(ctx context.Context, productID uuid.UUID) ([]*CompetitorProduct, error)
	GetByCompetitor(ctx context.Context, competitorID int) ([]*CompetitorProduct, error)
	BulkUpsert(ctx context.Context, links []*CompetitorProduct) (int, error)
	Count(ctx context.Context) (int64, error)
	CountByCompetitor(ctx context.Context) (map[int]int64, error)
}

// PriceObservationRepository defines the interface for price observations
type PriceObservationRepository interface {
	Create(ctx context.Context, observation *PriceObservation) error
	BulkCreate(ctx context.Context, observations []*PriceObservation) (int, error)
	GetLatestByProduct(ctx context.Context, productID uuid.UUID) ([]*PriceObservation, error)
	GetByProductAndCompetitor(ctx context.Context, productID uuid.UUID, competitorID int, since time.Time) ([]*PriceObservation, error)
	GetPriceHistory(ctx context.Context, productID uuid.UUID, days int) ([]*PriceObservation, error)
	Count(ctx context.Context) (int64, error)
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
}

// ImageRepository defines the interface for product images
type ImageRepository interface {
	Create(ctx context.Context, image *ProductImage) error
	GetByProduct(ctx context.Context, productID uuid.UUID) ([]*ProductImage, error)
	Update(ctx context.Context, image *ProductImage) error
	Delete(ctx context.Context, id uuid.UUID) error
	BulkUpsert(ctx context.Context, images []*ProductImage) (int, error)
}

// PropertyRepository defines the interface for product properties
type PropertyRepository interface {
	Create(ctx context.Context, property *ProductProperty) error
	GetByProduct(ctx context.Context, productID uuid.UUID) ([]*ProductProperty, error)
	BulkUpsert(ctx context.Context, properties []*ProductProperty) (int, error)
	DeleteByProduct(ctx context.Context, productID uuid.UUID) error
}

// SupplierRepository defines the interface for suppliers
type SupplierRepository interface {
	Create(ctx context.Context, supplier *Supplier) error
	GetByID(ctx context.Context, id string) (*Supplier, error)
	GetAll(ctx context.Context) ([]*Supplier, error)
	BulkUpsert(ctx context.Context, suppliers []*Supplier) (int, error)
}

// HistoryRepository defines the interface for operation history
type HistoryRepository interface {
	Add(ctx context.Context, entry *OperationHistory) error
	GetRecent(ctx context.Context, limit int) ([]*OperationHistory, error)
	GetByAction(ctx context.Context, action string, limit int) ([]*OperationHistory, error)
}

// QueryOptions represents options for list queries
type QueryOptions struct {
	Limit    int
	Offset   int
	OrderBy  string
	OrderDir string // "ASC" or "DESC"
	Vendor   string
	Status   models.ProductStatus
}

// Competitor represents a competitor in the database
type Competitor struct {
	ID             int               `json:"id"`
	Name           string            `json:"name"`
	NormalizedName string            `json:"normalized_name"`
	Website        string            `json:"website,omitempty"`
	ScrapeEnabled  bool              `json:"scrape_enabled"`
	ScrapeConfig   map[string]string `json:"scrape_config,omitempty"`
	ProductCount   int               `json:"product_count"`
	LastScraped    *time.Time        `json:"last_scraped,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// CompetitorProduct represents a link between a product and a competitor listing
type CompetitorProduct struct {
	ProductID       uuid.UUID `json:"product_id"`
	CompetitorID    int       `json:"competitor_id"`
	URL             string    `json:"url,omitempty"`
	CompetitorSKU   string    `json:"competitor_sku,omitempty"`
	CompetitorTitle string    `json:"competitor_title,omitempty"`
	IsActive        bool      `json:"is_active"`
	MatchMethod     string    `json:"match_method,omitempty"` // barcode, sku, title
	MatchConfidence float64   `json:"match_confidence"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// PriceObservation represents a single price observation
type PriceObservation struct {
	ID            int64     `json:"id,omitempty"`
	ProductID     uuid.UUID `json:"product_id"`
	CompetitorID  int       `json:"competitor_id"`
	Price         float64   `json:"price"`
	Currency      string    `json:"currency"`
	InStock       bool      `json:"in_stock"`
	StockQuantity *int      `json:"stock_quantity,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
	Source        string    `json:"source"` // reprice_csv, scraper, api
}

// ProductImage represents a product image in the database
type ProductImage struct {
	ID           uuid.UUID         `json:"id"`
	ProductID    uuid.UUID         `json:"product_id"`
	SourceURL    string            `json:"source_url"`
	Source       string            `json:"source"` // shopify, tiger_nl, nobb
	LocalPath    string            `json:"local_path,omitempty"`
	Width        int               `json:"width,omitempty"`
	Height       int               `json:"height,omitempty"`
	Position     int               `json:"position"`
	AltText      string            `json:"alt_text,omitempty"`
	Status       string            `json:"status"` // pending, downloaded, resized, uploaded, failed
	ResizedPaths map[string]string `json:"resized_paths,omitempty"`
	DownloadedAt *time.Time        `json:"downloaded_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

// ProductProperty represents a product property in the database
type ProductProperty struct {
	ProductID uuid.UUID `json:"product_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	Unit      string    `json:"unit,omitempty"`
	Source    string    `json:"source"` // nobb, tiger_nl, shopify
}

// Supplier represents a supplier in the database
type Supplier struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	GLN  string `json:"gln,omitempty"` // Global Location Number
}

// ProductSupplier represents a link between a product and a supplier
type ProductSupplier struct {
	ProductID     uuid.UUID `json:"product_id"`
	SupplierID    string    `json:"supplier_id"`
	ArticleNumber string    `json:"article_number,omitempty"`
	IsPrimary     bool      `json:"is_primary"`
}

// OperationHistory represents an operation in the history log
type OperationHistory struct {
	ID          int64      `json:"id,omitempty"`
	Action      string     `json:"action"`
	Source      string     `json:"source"`
	Count       int        `json:"count"`
	Details     string     `json:"details,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// EnhancementLog represents an enhancement action log entry
type EnhancementLog struct {
	ID          int64     `json:"id,omitempty"`
	ProductID   uuid.UUID `json:"product_id"`
	Source      string    `json:"source"`
	Action      string    `json:"action"`
	FieldsAdded []string  `json:"fields_added,omitempty"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
