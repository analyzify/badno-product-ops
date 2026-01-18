package shopify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/badno/badops/internal/output"
	"github.com/badno/badops/pkg/models"
)

const (
	AdapterName = "shopify"
	apiVersion  = "2024-01"
)

// Config holds Shopify output configuration
type Config struct {
	Store     string // Store name (e.g., "badno" for badno.myshopify.com)
	APIKey    string // API access token
	APIKeyEnv string // Environment variable name for API key
}

// Adapter implements the output.Adapter interface for Shopify
type Adapter struct {
	*output.BaseAdapter
	config  Config
	client  *http.Client
	baseURL string
}

// NewAdapter creates a new Shopify output adapter
func NewAdapter(cfg Config) *Adapter {
	return &Adapter{
		BaseAdapter: output.NewBaseAdapter(
			AdapterName,
			[]output.Format{}, // Shopify uses API, not file formats
		),
		config: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SupportsFormat - Shopify adapter doesn't use file formats
func (a *Adapter) SupportsFormat(format output.Format) bool {
	return false // We use API directly
}

// Connect establishes connection to Shopify API
func (a *Adapter) Connect(ctx context.Context) error {
	// Resolve API key from environment if needed
	apiKey := a.config.APIKey
	if apiKey == "" && a.config.APIKeyEnv != "" {
		apiKey = os.Getenv(a.config.APIKeyEnv)
	}
	if apiKey == "" {
		return fmt.Errorf("shopify API key not configured")
	}
	a.config.APIKey = apiKey

	// Build base URL
	store := a.config.Store
	if store == "" {
		return fmt.Errorf("shopify store name not configured")
	}
	a.baseURL = fmt.Sprintf("https://%s.myshopify.com/admin/api/%s", store, apiVersion)

	return a.Test(ctx)
}

// Close cleans up resources
func (a *Adapter) Close() error {
	a.SetConnected(false)
	return nil
}

// Test verifies connectivity to Shopify API
func (a *Adapter) Test(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", a.baseURL+"/shop.json", nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Shopify-Access-Token", a.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Shopify: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("shopify API error (status %d): %s", resp.StatusCode, string(body))
	}

	a.SetConnected(true)
	return nil
}

// ExportProducts updates products in Shopify
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
		result.Details = fmt.Sprintf("Dry run: would update %d products in Shopify", len(filteredProducts))
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Update each product
	updated := 0
	imagesAdded := 0
	var lastError error

	for _, p := range filteredProducts {
		if p.ID == "" {
			// Skip products without Shopify ID
			continue
		}

		newImages, err := a.updateProduct(ctx, p, opts)
		if err != nil {
			lastError = err
			continue
		}
		updated++
		imagesAdded += newImages
	}

	result.Destination = a.config.Store + ".myshopify.com"
	result.ProductsExported = updated
	result.ImagesExported = imagesAdded
	result.Success = lastError == nil
	result.Error = lastError
	result.Details = fmt.Sprintf("Updated %d/%d products in Shopify", updated, len(filteredProducts))
	result.CompletedAt = time.Now()

	return result, nil
}

// updateProduct updates a single product in Shopify
func (a *Adapter) updateProduct(ctx context.Context, product models.EnhancedProduct, opts output.ExportOptions) (int, error) {
	// Prepare update payload
	updateData := shopifyProductUpdate{
		Product: shopifyProduct{
			ID:          product.ID,
			Title:       product.Title,
			BodyHTML:    product.Description,
			Vendor:      product.Vendor,
			ProductType: product.ProductType,
		},
	}

	// Add new images if requested
	imagesAdded := 0
	if opts.IncludeImages {
		for _, img := range product.Images {
			// Only add new images (those with source other than shopify)
			if img.Source != "shopify" && img.Status != "existing" {
				updateData.Product.Images = append(updateData.Product.Images, shopifyImage{
					Src:      img.SourceURL,
					Position: img.Position,
					Alt:      img.Alt,
				})
				imagesAdded++
			}
		}
	}

	// Make API request
	body, err := json.Marshal(updateData)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("%s/products/%s.json", a.baseURL, product.ID)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}

	req.Header.Set("X-Shopify-Access-Token", a.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("shopify API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return imagesAdded, nil
}

// Shopify API types
type shopifyProductUpdate struct {
	Product shopifyProduct `json:"product"`
}

type shopifyProduct struct {
	ID          string         `json:"id,omitempty"`
	Title       string         `json:"title,omitempty"`
	BodyHTML    string         `json:"body_html,omitempty"`
	Vendor      string         `json:"vendor,omitempty"`
	ProductType string         `json:"product_type,omitempty"`
	Images      []shopifyImage `json:"images,omitempty"`
}

type shopifyImage struct {
	Src      string `json:"src"`
	Position int    `json:"position,omitempty"`
	Alt      string `json:"alt,omitempty"`
}
