package source

import (
	"context"

	"github.com/badno/badops/pkg/models"
)

// ConnectorType indicates whether a connector is a source or enhancement connector
type ConnectorType string

const (
	TypeSource      ConnectorType = "source"      // Primary data source (e.g., Shopify)
	TypeEnhancement ConnectorType = "enhancement" // Enrichment source (e.g., NOBB, Tiger.nl)
)

// Capability represents a specific feature a connector supports
type Capability string

const (
	CapabilityFetchProducts    Capability = "fetch_products"
	CapabilityFetchSingleProduct Capability = "fetch_single_product"
	CapabilityEnhanceProduct   Capability = "enhance_product"
	CapabilityFetchImages      Capability = "fetch_images"
	CapabilityFetchProperties  Capability = "fetch_properties"
	CapabilityFetchSuppliers   Capability = "fetch_suppliers"
	CapabilitySearch           Capability = "search"
)

// FetchOptions configures product fetching behavior
type FetchOptions struct {
	Limit       int               // Maximum products to fetch (0 = unlimited)
	Offset      int               // Starting offset for pagination
	Vendor      string            // Filter by vendor (e.g., "Tiger")
	SKUs        []string          // Specific SKUs to fetch
	UpdatedSince *int64           // Only fetch products updated after this Unix timestamp
	IncludeImages bool            // Include image data
	Filters     map[string]string // Additional filters
}

// EnhancementResult represents the result of enhancing a product
type EnhancementResult struct {
	Product       *models.EnhancedProduct
	FieldsUpdated []string
	ImagesAdded   int
	Success       bool
	Error         error
}

// FetchResult represents the result of fetching products
type FetchResult struct {
	Products    []models.EnhancedProduct
	TotalCount  int    // Total available (for pagination)
	HasMore     bool   // More products available
	NextCursor  string // Cursor for next page
}

// Connector defines the interface for data source connectors
type Connector interface {
	// Name returns the connector's unique identifier
	Name() string

	// Type returns whether this is a source or enhancement connector
	Type() ConnectorType

	// Capabilities returns the list of features this connector supports
	Capabilities() []Capability

	// Connect establishes a connection to the data source
	// This should validate credentials and connectivity
	Connect(ctx context.Context) error

	// Close cleans up any resources
	Close() error

	// FetchProducts retrieves products from the source
	// Only available for TypeSource connectors with CapabilityFetchProducts
	FetchProducts(ctx context.Context, opts FetchOptions) (*FetchResult, error)

	// EnhanceProduct enriches an existing product with additional data
	// Only available for TypeEnhancement connectors with CapabilityEnhanceProduct
	EnhanceProduct(ctx context.Context, product *models.EnhancedProduct) (*EnhancementResult, error)

	// Test performs a connectivity and credentials test
	Test(ctx context.Context) error
}

// HasCapability checks if a connector supports a specific capability
func HasCapability(c Connector, cap Capability) bool {
	for _, capability := range c.Capabilities() {
		if capability == cap {
			return true
		}
	}
	return false
}

// BaseConnector provides common functionality for connectors
type BaseConnector struct {
	name         string
	connectorType ConnectorType
	capabilities []Capability
	connected    bool
}

// NewBaseConnector creates a new base connector with common fields
func NewBaseConnector(name string, connType ConnectorType, caps []Capability) *BaseConnector {
	return &BaseConnector{
		name:         name,
		connectorType: connType,
		capabilities: caps,
	}
}

func (b *BaseConnector) Name() string {
	return b.name
}

func (b *BaseConnector) Type() ConnectorType {
	return b.connectorType
}

func (b *BaseConnector) Capabilities() []Capability {
	return b.capabilities
}

func (b *BaseConnector) IsConnected() bool {
	return b.connected
}

func (b *BaseConnector) SetConnected(connected bool) {
	b.connected = connected
}
