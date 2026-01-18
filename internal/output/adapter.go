package output

import (
	"context"
	"time"

	"github.com/badno/badops/pkg/models"
)

// Format specifies the output format
type Format string

const (
	FormatMatrixify Format = "matrixify" // Shopify Matrixify compatible CSV
	FormatShopify   Format = "shopify"   // Standard Shopify CSV
	FormatJSON      Format = "json"      // JSON format
	FormatJSONL     Format = "jsonl"     // JSON Lines format
)

// ExportOptions configures export behavior
type ExportOptions struct {
	Format       Format            // Output format
	OutputPath   string            // File path or destination
	IncludeImages bool             // Include image URLs
	OnlyEnhanced bool              // Only export enhanced products
	SKUs         []string          // Specific SKUs to export
	Filters      map[string]string // Additional filters
	DryRun       bool              // Preview without actually exporting
}

// ExportResult represents the result of an export operation
type ExportResult struct {
	Destination     string    // Where data was exported
	ProductsExported int      // Number of products exported
	ImagesExported  int       // Number of images exported
	Success         bool
	Error           error
	StartedAt       time.Time
	CompletedAt     time.Time
	Details         string    // Human-readable details
}

// Adapter defines the interface for output adapters
type Adapter interface {
	// Name returns the adapter's unique identifier
	Name() string

	// Connect establishes connection to the output destination
	Connect(ctx context.Context) error

	// Close cleans up any resources
	Close() error

	// ExportProducts exports products to the destination
	ExportProducts(ctx context.Context, products []models.EnhancedProduct, opts ExportOptions) (*ExportResult, error)

	// Test verifies connectivity to the destination
	Test(ctx context.Context) error

	// SupportsFormat checks if the adapter supports a specific format
	SupportsFormat(format Format) bool
}

// BaseAdapter provides common functionality for adapters
type BaseAdapter struct {
	name      string
	connected bool
	formats   []Format
}

// NewBaseAdapter creates a new base adapter
func NewBaseAdapter(name string, formats []Format) *BaseAdapter {
	return &BaseAdapter{
		name:    name,
		formats: formats,
	}
}

func (b *BaseAdapter) Name() string {
	return b.name
}

func (b *BaseAdapter) IsConnected() bool {
	return b.connected
}

func (b *BaseAdapter) SetConnected(connected bool) {
	b.connected = connected
}

func (b *BaseAdapter) SupportsFormat(format Format) bool {
	for _, f := range b.formats {
		if f == format {
			return true
		}
	}
	return false
}

func (b *BaseAdapter) SupportedFormats() []Format {
	return b.formats
}
