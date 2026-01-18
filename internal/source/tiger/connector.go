package tiger

import (
	"context"
	"fmt"
	"time"

	"github.com/badno/badops/internal/matcher"
	"github.com/badno/badops/internal/source"
	"github.com/badno/badops/pkg/models"
)

const ConnectorName = "tiger_nl"

// Config holds Tiger.nl connection configuration
type Config struct {
	RateLimitMs int // Milliseconds between requests (default: 150)
}

// Connector implements the source.Connector interface for Tiger.nl
type Connector struct {
	*source.BaseConnector
	config  Config
	matcher *matcher.TigerMatcher
}

// NewConnector creates a new Tiger.nl connector
func NewConnector(cfg Config) *Connector {
	if cfg.RateLimitMs <= 0 {
		cfg.RateLimitMs = 150
	}

	return &Connector{
		BaseConnector: source.NewBaseConnector(
			ConnectorName,
			source.TypeEnhancement,
			[]Capability{
				source.CapabilityEnhanceProduct,
				source.CapabilityFetchImages,
			},
		),
		config: cfg,
	}
}

type Capability = source.Capability

// Connect initializes the Tiger.nl matcher
func (c *Connector) Connect(ctx context.Context) error {
	c.matcher = matcher.NewTigerMatcher()
	c.SetConnected(true)
	return nil
}

// Close cleans up resources
func (c *Connector) Close() error {
	c.SetConnected(false)
	return nil
}

// Test verifies connectivity to Tiger.nl
func (c *Connector) Test(ctx context.Context) error {
	if c.matcher == nil {
		c.matcher = matcher.NewTigerMatcher()
	}

	// Test by making a simple lookup
	scraper := c.matcher.GetScraper()
	if scraper == nil {
		return fmt.Errorf("failed to initialize Tiger.nl scraper")
	}

	c.SetConnected(true)
	return nil
}

// FetchProducts is not supported for Tiger.nl (it's an enhancement source)
func (c *Connector) FetchProducts(ctx context.Context, opts source.FetchOptions) (*source.FetchResult, error) {
	return nil, fmt.Errorf("tiger_nl connector is an enhancement source, use EnhanceProduct instead")
}

// EnhanceProduct enriches a product with Tiger.nl images
func (c *Connector) EnhanceProduct(ctx context.Context, product *models.EnhancedProduct) (*source.EnhancementResult, error) {
	if !c.IsConnected() {
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
	}

	result := &source.EnhancementResult{
		Product: product,
		Success: false,
	}

	// Look up the product on Tiger.nl
	tigerProduct, err := c.matcher.LookupBySKU(product.SKU, product.Title)
	if err != nil {
		result.Error = fmt.Errorf("failed to lookup product: %w", err)
		return result, nil
	}

	if tigerProduct == nil {
		result.Error = fmt.Errorf("product not found on Tiger.nl")
		return result, nil
	}

	// Count existing images from Tiger.nl
	existingTigerImages := 0
	for _, img := range product.Images {
		if img.Source == "tiger_nl" {
			existingTigerImages++
		}
	}

	// Add new images
	newImagesAdded := 0
	existingCount := len(product.Images)

	for i, imgURL := range tigerProduct.ImageURLs {
		// Skip if we already have this many images from the source
		if i < existingTigerImages {
			continue
		}

		// Check if this URL is already in the product
		alreadyExists := false
		for _, existing := range product.Images {
			if existing.SourceURL == imgURL {
				alreadyExists = true
				break
			}
		}
		if alreadyExists {
			continue
		}

		// Add the new image
		product.Images = append(product.Images, models.ProductImage{
			SourceURL: imgURL,
			Position:  existingCount + newImagesAdded + 1,
			Status:    "pending",
			Source:    "tiger_nl",
		})
		newImagesAdded++
	}

	// Update legacy fields for backward compatibility
	product.LegacyMatchedURL = tigerProduct.URL
	product.LegacyMatchScore = 1.0 // Direct match

	// Record the enhancement
	if newImagesAdded > 0 {
		product.Enhancements = append(product.Enhancements, models.Enhancement{
			Source:      "tiger_nl",
			Action:      "images_added",
			Details:     fmt.Sprintf("Added %d new images from Tiger.nl (%s)", newImagesAdded, tigerProduct.URL),
			FieldsAdded: []string{"images"},
			Timestamp:   time.Now(),
			Success:     true,
		})
	} else {
		product.Enhancements = append(product.Enhancements, models.Enhancement{
			Source:    "tiger_nl",
			Action:    "images_checked",
			Details:   fmt.Sprintf("No new images found on Tiger.nl (%s)", tigerProduct.URL),
			Timestamp: time.Now(),
			Success:   true,
		})
	}

	product.UpdatedAt = time.Now()

	result.FieldsUpdated = []string{"images"}
	result.ImagesAdded = newImagesAdded
	result.Success = true

	return result, nil
}

// GetMatcher returns the underlying Tiger.nl matcher for direct access
func (c *Connector) GetMatcher() *matcher.TigerMatcher {
	return c.matcher
}
