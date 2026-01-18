package shopify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/badno/badops/internal/source"
	"github.com/badno/badops/pkg/models"
)

const (
	ConnectorName = "shopify"
	apiVersion    = "2024-01"
)

// Config holds Shopify connection configuration
type Config struct {
	Store      string // Store name (e.g., "badno" for badno.myshopify.com)
	APIKey     string // API access token
	APIKeyEnv  string // Environment variable name for API key
}

// Connector implements the source.Connector interface for Shopify
type Connector struct {
	*source.BaseConnector
	config  Config
	client  *http.Client
	baseURL string
}

// NewConnector creates a new Shopify connector
func NewConnector(cfg Config) *Connector {
	return &Connector{
		BaseConnector: source.NewBaseConnector(
			ConnectorName,
			source.TypeSource,
			[]Capability{
				source.CapabilityFetchProducts,
				source.CapabilityFetchSingleProduct,
				source.CapabilityFetchImages,
				source.CapabilitySearch,
			},
		),
		config: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type Capability = source.Capability

// Connect establishes connection to Shopify API
func (c *Connector) Connect(ctx context.Context) error {
	// Resolve API key from environment if needed
	apiKey := c.config.APIKey
	if apiKey == "" && c.config.APIKeyEnv != "" {
		apiKey = os.Getenv(c.config.APIKeyEnv)
	}
	if apiKey == "" {
		return fmt.Errorf("shopify API key not configured")
	}
	c.config.APIKey = apiKey

	// Build base URL
	store := c.config.Store
	if store == "" {
		return fmt.Errorf("shopify store name not configured")
	}
	c.baseURL = fmt.Sprintf("https://%s.myshopify.com/admin/api/%s", store, apiVersion)

	// Test connection
	return c.Test(ctx)
}

// Close cleans up resources
func (c *Connector) Close() error {
	c.SetConnected(false)
	return nil
}

// Test verifies connectivity to Shopify API
func (c *Connector) Test(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/shop.json", nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Shopify-Access-Token", c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Shopify: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("shopify API error (status %d): %s", resp.StatusCode, string(body))
	}

	c.SetConnected(true)
	return nil
}

// FetchProducts retrieves products from Shopify
func (c *Connector) FetchProducts(ctx context.Context, opts source.FetchOptions) (*source.FetchResult, error) {
	if !c.IsConnected() {
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
	}

	// Build query parameters
	params := url.Values{}
	if opts.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", min(opts.Limit, 250))) // Shopify max is 250
	} else {
		params.Set("limit", "250")
	}
	if opts.Vendor != "" {
		params.Set("vendor", opts.Vendor)
	}

	// Fetch products
	endpoint := c.baseURL + "/products.json?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Shopify-Access-Token", c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch products: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("shopify API error (status %d): %s", resp.StatusCode, string(body))
	}

	var shopifyResp shopifyProductsResponse
	if err := json.NewDecoder(resp.Body).Decode(&shopifyResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for pagination
	linkHeader := resp.Header.Get("Link")
	hasMore := strings.Contains(linkHeader, `rel="next"`)
	nextCursor := extractNextCursor(linkHeader)

	// Convert to EnhancedProduct
	products := make([]models.EnhancedProduct, 0, len(shopifyResp.Products))
	for _, sp := range shopifyResp.Products {
		products = append(products, convertShopifyProduct(sp))
	}

	return &source.FetchResult{
		Products:   products,
		TotalCount: len(products),
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, nil
}

// EnhanceProduct is not supported for Shopify source connector
func (c *Connector) EnhanceProduct(ctx context.Context, product *models.EnhancedProduct) (*source.EnhancementResult, error) {
	return nil, fmt.Errorf("shopify connector does not support product enhancement")
}

// Shopify API response types
type shopifyProductsResponse struct {
	Products []shopifyProduct `json:"products"`
}

type shopifyProduct struct {
	ID          int64            `json:"id"`
	Title       string           `json:"title"`
	Handle      string           `json:"handle"`
	BodyHTML    string           `json:"body_html"`
	Vendor      string           `json:"vendor"`
	ProductType string           `json:"product_type"`
	Tags        string           `json:"tags"`
	Status      string           `json:"status"`
	CreatedAt   string           `json:"created_at"`
	UpdatedAt   string           `json:"updated_at"`
	Variants    []shopifyVariant `json:"variants"`
	Images      []shopifyImage   `json:"images"`
}

type shopifyVariant struct {
	ID                int64   `json:"id"`
	ProductID         int64   `json:"product_id"`
	SKU               string  `json:"sku"`
	Barcode           string  `json:"barcode"`
	Price             string  `json:"price"`
	CompareAtPrice    string  `json:"compare_at_price"`
	Weight            float64 `json:"weight"`
	WeightUnit        string  `json:"weight_unit"`
	InventoryQuantity int     `json:"inventory_quantity"`
}

type shopifyImage struct {
	ID        int64  `json:"id"`
	Src       string `json:"src"`
	Position  int    `json:"position"`
	Alt       string `json:"alt"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// convertShopifyProduct converts a Shopify product to EnhancedProduct
func convertShopifyProduct(sp shopifyProduct) models.EnhancedProduct {
	ep := models.EnhancedProduct{
		ID:          fmt.Sprintf("%d", sp.ID),
		Handle:      sp.Handle,
		Title:       sp.Title,
		Description: sp.BodyHTML,
		Vendor:      sp.Vendor,
		ProductType: sp.ProductType,
		Status:      models.StatusPending,
		Specifications: make(map[string]string),
	}

	// Parse tags
	if sp.Tags != "" {
		ep.Tags = strings.Split(sp.Tags, ", ")
	}

	// Use first variant for SKU and pricing
	if len(sp.Variants) > 0 {
		v := sp.Variants[0]
		ep.SKU = v.SKU
		ep.Barcode = v.Barcode

		// Parse price
		var price float64
		fmt.Sscanf(v.Price, "%f", &price)
		var compareAt float64
		fmt.Sscanf(v.CompareAtPrice, "%f", &compareAt)

		ep.Price = &models.Price{
			Amount:    price,
			Currency:  "NOK", // Default to NOK for bad.no
			CompareAt: compareAt,
		}

		// Weight
		if v.Weight > 0 {
			ep.Weight = &models.Weight{
				Value: v.Weight,
				Unit:  v.WeightUnit,
			}
		}
	}

	// Convert images
	for _, img := range sp.Images {
		ep.Images = append(ep.Images, models.ProductImage{
			ID:        fmt.Sprintf("%d", img.ID),
			SourceURL: img.Src,
			Position:  img.Position,
			Alt:       img.Alt,
			Width:     img.Width,
			Height:    img.Height,
			Status:    "existing",
			Source:    "shopify",
		})
	}

	// Parse timestamps
	if t, err := time.Parse(time.RFC3339, sp.CreatedAt); err == nil {
		ep.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, sp.UpdatedAt); err == nil {
		ep.UpdatedAt = t
	}

	return ep
}

// extractNextCursor extracts the next page cursor from Link header
func extractNextCursor(linkHeader string) string {
	// Link header format: <url>; rel="next", <url>; rel="previous"
	parts := strings.Split(linkHeader, ",")
	for _, part := range parts {
		if strings.Contains(part, `rel="next"`) {
			// Extract URL between < and >
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start >= 0 && end > start {
				nextURL := part[start+1 : end]
				// Extract page_info parameter
				if u, err := url.Parse(nextURL); err == nil {
					return u.Query().Get("page_info")
				}
			}
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
