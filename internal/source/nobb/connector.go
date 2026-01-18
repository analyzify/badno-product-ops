package nobb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/badno/badops/internal/source"
	"github.com/badno/badops/pkg/models"
)

const (
	ConnectorName = "nobb"
	baseURL       = "https://export.byggtjeneste.no/api"
)

// Config holds NOBB connection configuration
type Config struct {
	Username    string // NOBB username
	Password    string // NOBB password
	UsernameEnv string // Environment variable for username
	PasswordEnv string // Environment variable for password
}

// Connector implements the source.Connector interface for NOBB
type Connector struct {
	*source.BaseConnector
	config    Config
	client    *http.Client
	authToken string
}

// NewConnector creates a new NOBB connector
func NewConnector(cfg Config) *Connector {
	return &Connector{
		BaseConnector: source.NewBaseConnector(
			ConnectorName,
			source.TypeEnhancement,
			[]Capability{
				source.CapabilityEnhanceProduct,
				source.CapabilityFetchProperties,
				source.CapabilityFetchSuppliers,
				source.CapabilitySearch,
			},
		),
		config: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type Capability = source.Capability

// Connect establishes connection to NOBB API
func (c *Connector) Connect(ctx context.Context) error {
	// Resolve credentials from environment
	username := c.config.Username
	if username == "" && c.config.UsernameEnv != "" {
		username = os.Getenv(c.config.UsernameEnv)
	}
	password := c.config.Password
	if password == "" && c.config.PasswordEnv != "" {
		password = os.Getenv(c.config.PasswordEnv)
	}

	if username == "" || password == "" {
		return fmt.Errorf("NOBB credentials not configured")
	}

	c.config.Username = username
	c.config.Password = password

	// Create Basic Auth token
	c.authToken = base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%s:%s", username, password)),
	)

	return c.Test(ctx)
}

// Close cleans up resources
func (c *Connector) Close() error {
	c.SetConnected(false)
	return nil
}

// Test verifies connectivity to NOBB API
func (c *Connector) Test(ctx context.Context) error {
	// Test with a simple items request (limit 1)
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/items?limit=1", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Basic "+c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to NOBB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("NOBB authentication failed: invalid credentials")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("NOBB API error (status %d): %s", resp.StatusCode, string(body))
	}

	c.SetConnected(true)
	return nil
}

// FetchProducts is not the primary use case for NOBB (it's an enhancement source)
func (c *Connector) FetchProducts(ctx context.Context, opts source.FetchOptions) (*source.FetchResult, error) {
	return nil, fmt.Errorf("NOBB connector is an enhancement source, use EnhanceProduct instead")
}

// EnhanceProduct enriches a product with NOBB data
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

	// Try to find by NOBB number if available
	var nobbItem *nobbItem
	var err error

	if product.NOBBNumber != "" {
		nobbItem, err = c.fetchItemByNOBBNumber(ctx, product.NOBBNumber)
	}

	// Fall back to searching by EAN/barcode
	if nobbItem == nil && product.Barcode != "" {
		nobbItem, err = c.searchItemByEAN(ctx, product.Barcode)
	}

	// Fall back to searching by SKU
	if nobbItem == nil && product.SKU != "" {
		nobbItem, err = c.searchItemBySKU(ctx, product.SKU)
	}

	if err != nil {
		result.Error = err
		return result, nil
	}

	if nobbItem == nil {
		result.Error = fmt.Errorf("product not found in NOBB database")
		return result, nil
	}

	// Apply enhancements
	fieldsUpdated := c.applyNobbData(product, nobbItem)

	// Fetch additional data
	if nobbItem.NobbNumber != "" {
		// Fetch properties
		props, _ := c.fetchProperties(ctx, nobbItem.NobbNumber)
		if len(props) > 0 {
			for _, p := range props {
				product.Properties = append(product.Properties, models.Property{
					Code:   p.PropertyCode,
					Name:   p.PropertyName,
					Value:  p.Value,
					Unit:   p.Unit,
					Source: "nobb",
				})
			}
			fieldsUpdated = append(fieldsUpdated, "properties")
		}

		// Fetch suppliers
		suppliers, _ := c.fetchSuppliers(ctx, nobbItem.NobbNumber)
		if len(suppliers) > 0 {
			for i, s := range suppliers {
				product.Suppliers = append(product.Suppliers, models.Supplier{
					ID:        s.SupplierID,
					Name:      s.SupplierName,
					GLN:       s.GLN,
					ArticleNo: s.SupplierArticleNo,
					IsPrimary: i == 0,
				})
			}
			fieldsUpdated = append(fieldsUpdated, "suppliers")
		}

		// Fetch package info
		packages, _ := c.fetchPackages(ctx, nobbItem.NobbNumber)
		if len(packages) > 0 {
			for _, p := range packages {
				product.PackageInfo = append(product.PackageInfo, models.PackageInfo{
					Type:       p.PackageType,
					Quantity:   p.Quantity,
					GTIN:       p.GTIN,
					Weight:     p.Weight,
					WeightUnit: p.WeightUnit,
					Length:     p.Length,
					Width:      p.Width,
					Height:     p.Height,
					DimUnit:    p.DimensionUnit,
				})
			}
			fieldsUpdated = append(fieldsUpdated, "package_info")
		}
	}

	// Record the enhancement
	product.Enhancements = append(product.Enhancements, models.Enhancement{
		Source:      "nobb",
		Action:      "data_enriched",
		Details:     fmt.Sprintf("Enhanced with NOBB data (NOBB#: %s)", nobbItem.NobbNumber),
		FieldsAdded: fieldsUpdated,
		Timestamp:   time.Now(),
		Success:     true,
	})

	product.UpdatedAt = time.Now()
	result.FieldsUpdated = fieldsUpdated
	result.Success = true

	return result, nil
}

// NOBB API response types
type nobbItem struct {
	NobbNumber       string  `json:"nobbNumber"`
	ProductName      string  `json:"productName"`
	Description      string  `json:"description"`
	ProductGroupCode string  `json:"productGroupCode"`
	ProductGroupName string  `json:"productGroupName"`
	Status           string  `json:"status"`
	Weight           float64 `json:"weight"`
	WeightUnit       string  `json:"weightUnit"`
	Length           float64 `json:"length"`
	Width            float64 `json:"width"`
	Height           float64 `json:"height"`
	DimensionUnit    string  `json:"dimensionUnit"`
}

type nobbProperty struct {
	PropertyCode string `json:"propertyCode"`
	PropertyName string `json:"propertyName"`
	Value        string `json:"value"`
	Unit         string `json:"unit"`
}

type nobbSupplier struct {
	SupplierID        string `json:"supplierId"`
	SupplierName      string `json:"supplierName"`
	GLN               string `json:"gln"`
	SupplierArticleNo string `json:"supplierArticleNo"`
}

type nobbPackage struct {
	PackageType   string  `json:"packageType"`
	Quantity      int     `json:"quantity"`
	GTIN          string  `json:"gtin"`
	Weight        float64 `json:"weight"`
	WeightUnit    string  `json:"weightUnit"`
	Length        float64 `json:"length"`
	Width         float64 `json:"width"`
	Height        float64 `json:"height"`
	DimensionUnit string  `json:"dimensionUnit"`
}

type nobbItemsResponse struct {
	Items         []nobbItem `json:"items"`
	ForwardToken  string     `json:"forwardToken"`
}

// fetchItemByNOBBNumber fetches a single item by NOBB number
func (c *Connector) fetchItemByNOBBNumber(ctx context.Context, nobbNumber string) (*nobbItem, error) {
	url := fmt.Sprintf("%s/items/%s", baseURL, nobbNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Basic "+c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NOBB API error: status %d", resp.StatusCode)
	}

	var item nobbItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, err
	}

	return &item, nil
}

// searchItemByEAN searches for an item by EAN code
func (c *Connector) searchItemByEAN(ctx context.Context, ean string) (*nobbItem, error) {
	url := fmt.Sprintf("%s/items?ean=%s&limit=1", baseURL, ean)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Basic "+c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NOBB API error: status %d", resp.StatusCode)
	}

	var response nobbItemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	if len(response.Items) == 0 {
		return nil, nil
	}

	return &response.Items[0], nil
}

// searchItemBySKU searches for an item by supplier article number (SKU)
func (c *Connector) searchItemBySKU(ctx context.Context, sku string) (*nobbItem, error) {
	url := fmt.Sprintf("%s/items?supplierArticleNo=%s&limit=1", baseURL, sku)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Basic "+c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NOBB API error: status %d", resp.StatusCode)
	}

	var response nobbItemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	if len(response.Items) == 0 {
		return nil, nil
	}

	return &response.Items[0], nil
}

// fetchProperties fetches properties for a NOBB item
func (c *Connector) fetchProperties(ctx context.Context, nobbNumber string) ([]nobbProperty, error) {
	url := fmt.Sprintf("%s/items/%s/properties", baseURL, nobbNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Basic "+c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var properties []nobbProperty
	if err := json.NewDecoder(resp.Body).Decode(&properties); err != nil {
		return nil, err
	}

	return properties, nil
}

// fetchSuppliers fetches suppliers for a NOBB item
func (c *Connector) fetchSuppliers(ctx context.Context, nobbNumber string) ([]nobbSupplier, error) {
	url := fmt.Sprintf("%s/items/%s/suppliers", baseURL, nobbNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Basic "+c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var suppliers []nobbSupplier
	if err := json.NewDecoder(resp.Body).Decode(&suppliers); err != nil {
		return nil, err
	}

	return suppliers, nil
}

// fetchPackages fetches package info for a NOBB item
func (c *Connector) fetchPackages(ctx context.Context, nobbNumber string) ([]nobbPackage, error) {
	url := fmt.Sprintf("%s/items/%s/packages", baseURL, nobbNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Basic "+c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var packages []nobbPackage
	if err := json.NewDecoder(resp.Body).Decode(&packages); err != nil {
		return nil, err
	}

	return packages, nil
}

// applyNobbData applies NOBB data to an EnhancedProduct
func (c *Connector) applyNobbData(product *models.EnhancedProduct, item *nobbItem) []string {
	var fieldsUpdated []string

	// Set NOBB number
	if product.NOBBNumber == "" && item.NobbNumber != "" {
		product.NOBBNumber = item.NobbNumber
		fieldsUpdated = append(fieldsUpdated, "nobb_number")
	}

	// Update description if not set
	if product.Description == "" && item.Description != "" {
		product.Description = item.Description
		fieldsUpdated = append(fieldsUpdated, "description")
	}

	// Set product type from NOBB category
	if product.ProductType == "" && item.ProductGroupName != "" {
		product.ProductType = item.ProductGroupName
		fieldsUpdated = append(fieldsUpdated, "product_type")
	}

	// Set dimensions
	if product.Dimensions == nil && (item.Length > 0 || item.Width > 0 || item.Height > 0) {
		product.Dimensions = &models.Dimensions{
			Length: item.Length,
			Width:  item.Width,
			Height: item.Height,
			Unit:   item.DimensionUnit,
		}
		fieldsUpdated = append(fieldsUpdated, "dimensions")
	}

	// Set weight
	if product.Weight == nil && item.Weight > 0 {
		product.Weight = &models.Weight{
			Value: item.Weight,
			Unit:  item.WeightUnit,
		}
		fieldsUpdated = append(fieldsUpdated, "weight")
	}

	// Add to specifications
	if product.Specifications == nil {
		product.Specifications = make(map[string]string)
	}
	if item.ProductGroupCode != "" {
		product.Specifications["nobb_product_group_code"] = item.ProductGroupCode
		product.Specifications["nobb_product_group_name"] = item.ProductGroupName
	}

	return fieldsUpdated
}
