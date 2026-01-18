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
	baseURL       = "https://export.byggtjeneste.no/api/v1"
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
	// Ensure credentials are set
	if c.authToken == "" {
		// Try to resolve credentials
		username := c.config.Username
		if username == "" && c.config.UsernameEnv != "" {
			username = os.Getenv(c.config.UsernameEnv)
		}
		password := c.config.Password
		if password == "" && c.config.PasswordEnv != "" {
			password = os.Getenv(c.config.PasswordEnv)
		}

		if username == "" || password == "" {
			return fmt.Errorf("NOBB credentials not configured (set %s and %s environment variables)",
				c.config.UsernameEnv, c.config.PasswordEnv)
		}

		c.authToken = base64.StdEncoding.EncodeToString(
			[]byte(fmt.Sprintf("%s:%s", username, password)),
		)
	}

	// Test with a simple items request (limit 1)
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/items?pageSize=1", nil)
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
	var searchMethod string

	if product.NOBBNumber != "" {
		searchMethod = "nobb_number"
		nobbItem, err = c.fetchItemByNOBBNumber(ctx, product.NOBBNumber)
		if err != nil {
			result.Error = fmt.Errorf("NOBB search by number failed: %w", err)
			return result, nil
		}
	}

	// Fall back to searching by EAN/barcode
	if nobbItem == nil && product.Barcode != "" {
		searchMethod = "barcode"
		nobbItem, err = c.searchItemByEAN(ctx, product.Barcode)
		if err != nil {
			result.Error = fmt.Errorf("NOBB search by EAN failed: %w", err)
			return result, nil
		}
	}

	// Fall back to searching by SKU
	if nobbItem == nil && product.SKU != "" {
		searchMethod = "sku"
		nobbItem, err = c.searchItemBySKU(ctx, product.SKU)
		if err != nil {
			result.Error = fmt.Errorf("NOBB search by SKU failed: %w", err)
			return result, nil
		}
	}

	if nobbItem == nil {
		details := fmt.Sprintf("product not found in NOBB database (searched by: %s", searchMethod)
		if product.NOBBNumber != "" {
			details += fmt.Sprintf(", nobb=%s", product.NOBBNumber)
		}
		if product.Barcode != "" {
			details += fmt.Sprintf(", barcode=%s", product.Barcode)
		}
		if product.SKU != "" {
			details += fmt.Sprintf(", sku=%s", product.SKU)
		}
		details += ")"
		result.Error = fmt.Errorf(details)
		return result, nil
	}

	// Fetch additional properties if not included in main response
	nobbNumStr := fmt.Sprintf("%d", nobbItem.NobbNumber)
	if nobbItem.Properties == nil || nobbItem.Properties.IsEmpty() {
		props, propErr := c.fetchPropertiesSeparate(ctx, nobbNumStr)
		if propErr == nil && len(props) > 0 {
			if nobbItem.Properties == nil {
				nobbItem.Properties = &nobbPropertiesGroup{}
			}
			nobbItem.Properties.Other = append(nobbItem.Properties.Other, props...)
		}
	}

	// Apply enhancements (suppliers, packages, media, and properties)
	fieldsUpdated := c.applyNobbData(product, nobbItem)

	// Build enhancement details
	var details []string
	details = append(details, fmt.Sprintf("NOBB#: %s", nobbNumStr))
	propCount := nobbItem.Properties.TotalCount()
	if propCount > 0 {
		details = append(details, fmt.Sprintf("%d properties", propCount))
	}
	if len(product.PackageInfo) > 0 {
		details = append(details, fmt.Sprintf("%d packages", len(product.PackageInfo)))
	}
	imageCount := 0
	for _, img := range product.Images {
		if img.Source == "nobb" {
			imageCount++
		}
	}
	if imageCount > 0 {
		details = append(details, fmt.Sprintf("%d images", imageCount))
	}

	// Record the enhancement
	product.Enhancements = append(product.Enhancements, models.Enhancement{
		Source:      "nobb",
		Action:      "data_enriched",
		Details:     fmt.Sprintf("Enhanced with NOBB data (%s)", joinDetails(details)),
		FieldsAdded: fieldsUpdated,
		Timestamp:   time.Now(),
		Success:     true,
	})

	product.UpdatedAt = time.Now()
	result.FieldsUpdated = fieldsUpdated
	result.Success = true

	return result, nil
}

// joinDetails joins details with comma separator
func joinDetails(details []string) string {
	if len(details) == 0 {
		return ""
	}
	result := details[0]
	for i := 1; i < len(details); i++ {
		result += ", " + details[i]
	}
	return result
}

// NOBB API response types - matches actual API v1 response
type nobbItem struct {
	NobbNumber              int                  `json:"nobbNumber"`
	PrimaryText             string               `json:"primaryText"`
	SecondaryText           string               `json:"secondaryText"`
	Description             string               `json:"description"`
	ProductGroupNum         string               `json:"productGroupNumber"`
	ProductGroupName        string               `json:"productGroupName"`
	CountryOfOrigin         string               `json:"countryOfOrigin"`
	CustomsCode             string               `json:"customsNoCode"` // Norwegian customs code
	CustomsCodeEU           string               `json:"customsEuCode"` // EU customs code
	EtimClass               string               `json:"etimClass"`
	ManufacturerItemNumber  string               `json:"manufacturerItemNumber"`
	NrfInfo                 *nobbNrfInfo         `json:"nrfInfo"`
	DigitalChannelText      string               `json:"digitalChannelText"`
	Suppliers               []nobbSupplier       `json:"suppliers"`
	Properties              *nobbPropertiesGroup `json:"properties"` // Properties grouped by category
}

// nobbPropertiesGroup contains properties categorized by type
type nobbPropertiesGroup struct {
	ETIM        []nobbProperty `json:"etim"`
	Environment []nobbProperty `json:"environment"`
	Marketing   []nobbProperty `json:"marketing"`
	EPD         []nobbProperty `json:"epd"`
	Other       []nobbProperty `json:"other"`
}

// IsEmpty returns true if all property categories are empty
func (g *nobbPropertiesGroup) IsEmpty() bool {
	if g == nil {
		return true
	}
	return len(g.ETIM) == 0 && len(g.Environment) == 0 && len(g.Marketing) == 0 && len(g.EPD) == 0 && len(g.Other) == 0
}

// TotalCount returns the total number of properties across all categories
func (g *nobbPropertiesGroup) TotalCount() int {
	if g == nil {
		return 0
	}
	return len(g.ETIM) + len(g.Environment) + len(g.Marketing) + len(g.EPD) + len(g.Other)
}

// nobbNrfInfo contains NRF (Norwegian Retail Federation) information
type nobbNrfInfo struct {
	NrfNumber   string `json:"nrfNumber"`
	NrfName     string `json:"nrfName"`
	NrfGroup    string `json:"nrfGroup"`
	NrfSubGroup string `json:"nrfSubGroup"`
}

type nobbSupplier struct {
	ParticipantNumber string        `json:"participantNumber"`
	Name              string        `json:"name"`
	IsMainSupplier    bool          `json:"isMainSupplier"`
	SupplierItemNum   string        `json:"supplierItemNumber"`
	Packages          []nobbPackage `json:"packages"`
	Media             []nobbMedia   `json:"media"`
}

type nobbPackage struct {
	Class           string  `json:"class"`            // PIECE, INNER, OUTER, PALLET
	GTIN            string  `json:"gtin"`             // Barcode (GTIN-13)
	Weight          float64 `json:"weight"`           // Weight in kg
	Length          int     `json:"length"`           // Length in mm
	Width           int     `json:"width"`            // Width in mm
	Height          int     `json:"height"`           // Height in mm
	Volume          float64 `json:"volume"`           // Volume in liters
	Unit            string  `json:"unit"`             // Unit code
	IsPCU           bool    `json:"isPCU"`            // Is Price Calculation Unit
	MinOrderQty     int     `json:"minOrderQuantity"` // Minimum order quantity
	Deliverable     bool    `json:"deliverable"`      // Can be delivered
	Stocked         bool    `json:"stocked"`          // Is stocked
	CalculatedCount float64 `json:"calculatedCount"`  // Calculated count
	ConsistsOfCount float64 `json:"consistsOfCount"`  // Number of items contained (float from API)
	ConsistsOfUnit  string  `json:"consistsOfUnit"`   // Unit of contained items
	DangerousGoods  bool    `json:"dangerousGoods"`   // Contains dangerous goods
	DGUNNumber      string  `json:"dgunNumber"`       // UN number for dangerous goods
}

type nobbMedia struct {
	GUID      string `json:"guid"`
	MediaType string `json:"mediaType"`
	URL       string `json:"url"`
	IsPrimary bool   `json:"isPrimary"`
}

// nobbProperty for properties endpoint (separate call)
// nobbProperty represents a property from any category
type nobbProperty struct {
	PropertyGUID        string `json:"propertyGuid"`
	PropertyCode        string `json:"propertyCode"`        // Used in separate /properties endpoint
	PropertyName        string `json:"propertyName"`
	PropertyDescription string `json:"propertyDescription"`
	Value               string `json:"value"`
	Unit                string `json:"unit"`
}

// fetchItemByNOBBNumber fetches a single item by NOBB number using GET with nobbnos param
func (c *Connector) fetchItemByNOBBNumber(ctx context.Context, nobbNumber string) (*nobbItem, error) {
	url := fmt.Sprintf("%s/items?nobbnos=%s", baseURL, nobbNumber)
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
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("NOBB API error: status %d - %s", resp.StatusCode, string(respBody))
	}

	// API returns array directly
	var items []nobbItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("JSON decode error: %w", err)
	}

	if len(items) == 0 {
		return nil, nil
	}

	return &items[0], nil
}

// searchItemByEAN searches for an item by EAN/GTIN code
func (c *Connector) searchItemByEAN(ctx context.Context, ean string) (*nobbItem, error) {
	// Use GET /items?gtins=XXXXX
	url := fmt.Sprintf("%s/items?gtins=%s", baseURL, ean)
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
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("NOBB API error: status %d - %s", resp.StatusCode, string(respBody))
	}

	// API returns array directly
	var items []nobbItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, nil
	}

	return &items[0], nil
}

// searchItemBySKU searches for an item by supplier article number (SKU)
// Note: NOBB API doesn't directly support searching by supplier article number.
// This function attempts to search by NOBB number if the SKU follows NOBB format.
func (c *Connector) searchItemBySKU(ctx context.Context, sku string) (*nobbItem, error) {
	// If SKU looks like a NOBB number (8 digits), try to fetch it directly
	if len(sku) == 8 && isNumeric(sku) {
		return c.fetchItemByNOBBNumber(ctx, sku)
	}

	// Strip common prefixes and try again
	// e.g., CO-T309012 -> T309012 -> 309012
	cleanSKU := sku
	for _, prefix := range []string{"CO-T", "CO-", "T"} {
		if len(cleanSKU) > len(prefix) && cleanSKU[:len(prefix)] == prefix {
			cleanSKU = cleanSKU[len(prefix):]
			break
		}
	}

	// If cleaned SKU looks like a NOBB number, try that
	if len(cleanSKU) >= 6 && len(cleanSKU) <= 8 && isNumeric(cleanSKU) {
		// Pad to 8 digits if needed
		for len(cleanSKU) < 8 {
			cleanSKU = "0" + cleanSKU
		}
		return c.fetchItemByNOBBNumber(ctx, cleanSKU)
	}

	// NOBB API doesn't support direct supplier article number search
	return nil, nil
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// fetchProperties fetches properties for a NOBB item
// fetchPropertiesSeparate fetches properties from the separate /properties endpoint
// This endpoint returns a flat list of properties, different from the main items endpoint
func (c *Connector) fetchPropertiesSeparate(ctx context.Context, nobbNumber string) ([]nobbProperty, error) {
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
	nobbNumStr := fmt.Sprintf("%d", item.NobbNumber)
	if product.NOBBNumber == "" && item.NobbNumber > 0 {
		product.NOBBNumber = nobbNumStr
		fieldsUpdated = append(fieldsUpdated, "nobb_number")
	}

	// Update description if not set
	if product.Description == "" && item.Description != "" {
		product.Description = item.Description
		fieldsUpdated = append(fieldsUpdated, "description")
	}

	// Use digital channel text for description if available
	if product.Description == "" && item.DigitalChannelText != "" {
		product.Description = item.DigitalChannelText
		fieldsUpdated = append(fieldsUpdated, "description")
	}

	// Set product type from NOBB category
	if product.ProductType == "" && item.ProductGroupName != "" {
		product.ProductType = item.ProductGroupName
		fieldsUpdated = append(fieldsUpdated, "product_type")
	}

	// Extract dimensions and weight from first supplier's package info
	if len(item.Suppliers) > 0 && len(item.Suppliers[0].Packages) > 0 {
		pkg := item.Suppliers[0].Packages[0]

		// Set dimensions (values are in mm)
		if product.Dimensions == nil && (pkg.Length > 0 || pkg.Width > 0 || pkg.Height > 0) {
			product.Dimensions = &models.Dimensions{
				Length: float64(pkg.Length),
				Width:  float64(pkg.Width),
				Height: float64(pkg.Height),
				Unit:   "mm",
			}
			fieldsUpdated = append(fieldsUpdated, "dimensions")
		}

		// Set weight (value is in kg)
		if product.Weight == nil && pkg.Weight > 0 {
			product.Weight = &models.Weight{
				Value: pkg.Weight,
				Unit:  "kg",
			}
			fieldsUpdated = append(fieldsUpdated, "weight")
		}

		// Set barcode from GTIN if not already set
		if product.Barcode == "" && pkg.GTIN != "" {
			product.Barcode = pkg.GTIN
			fieldsUpdated = append(fieldsUpdated, "barcode")
		}
	}

	// Initialize specifications map
	if product.Specifications == nil {
		product.Specifications = make(map[string]string)
	}

	// Add core NOBB specifications
	if item.ProductGroupNum != "" {
		product.Specifications["nobb_product_group_code"] = item.ProductGroupNum
		product.Specifications["nobb_product_group_name"] = item.ProductGroupName
		product.Specifications["nobb_primary_text"] = item.PrimaryText
		if item.SecondaryText != "" {
			product.Specifications["nobb_secondary_text"] = item.SecondaryText
		}
		if item.CountryOfOrigin != "" {
			product.Specifications["country_of_origin"] = item.CountryOfOrigin
		}
	}

	// Add extended NOBB fields
	if item.CustomsCode != "" {
		product.Specifications["customs_code"] = item.CustomsCode
	}
	if item.EtimClass != "" {
		product.Specifications["etim_class"] = item.EtimClass
	}
	if item.ManufacturerItemNumber != "" {
		product.Specifications["manufacturer_item_number"] = item.ManufacturerItemNumber
	}

	// Add NRF info if available
	if item.NrfInfo != nil {
		if item.NrfInfo.NrfNumber != "" {
			product.Specifications["nrf_number"] = item.NrfInfo.NrfNumber
		}
		if item.NrfInfo.NrfName != "" {
			product.Specifications["nrf_name"] = item.NrfInfo.NrfName
		}
		if item.NrfInfo.NrfGroup != "" {
			product.Specifications["nrf_group"] = item.NrfInfo.NrfGroup
		}
		if item.NrfInfo.NrfSubGroup != "" {
			product.Specifications["nrf_sub_group"] = item.NrfInfo.NrfSubGroup
		}
	}

	// Extract properties from all categories (if included in main response)
	if item.Properties != nil {
		// Extract ETIM properties (technical specifications)
		for _, prop := range item.Properties.ETIM {
			product.Properties = append(product.Properties, models.Property{
				Code:   prop.PropertyGUID,
				Name:   prop.PropertyName,
				Value:  prop.Value,
				Unit:   prop.Unit,
				Source: "nobb_etim",
			})
		}
		// Extract environment properties
		for _, prop := range item.Properties.Environment {
			product.Properties = append(product.Properties, models.Property{
				Code:   prop.PropertyGUID,
				Name:   prop.PropertyName,
				Value:  prop.Value,
				Unit:   prop.Unit,
				Source: "nobb_env",
			})
		}
		// Extract marketing properties
		for _, prop := range item.Properties.Marketing {
			product.Properties = append(product.Properties, models.Property{
				Code:   prop.PropertyGUID,
				Name:   prop.PropertyName,
				Value:  prop.Value,
				Unit:   prop.Unit,
				Source: "nobb_marketing",
			})
		}
		// Extract EPD (Environmental Product Declaration) properties
		for _, prop := range item.Properties.EPD {
			product.Properties = append(product.Properties, models.Property{
				Code:   prop.PropertyGUID,
				Name:   prop.PropertyName,
				Value:  prop.Value,
				Unit:   prop.Unit,
				Source: "nobb_epd",
			})
		}
		// Extract other properties
		for _, prop := range item.Properties.Other {
			product.Properties = append(product.Properties, models.Property{
				Code:   prop.PropertyGUID,
				Name:   prop.PropertyName,
				Value:  prop.Value,
				Unit:   prop.Unit,
				Source: "nobb_other",
			})
		}
		if len(product.Properties) > 0 {
			fieldsUpdated = append(fieldsUpdated, "properties")
		}
	}

	// Extract supplier info with media
	for _, sup := range item.Suppliers {
		product.Suppliers = append(product.Suppliers, models.Supplier{
			ID:        sup.ParticipantNumber,
			Name:      sup.Name,
			ArticleNo: sup.SupplierItemNum,
			IsPrimary: sup.IsMainSupplier,
		})

		// Extract images from supplier media
		for _, media := range sup.Media {
			if media.URL != "" {
				// Determine position - primary images come first
				position := len(product.Images) + 1
				if media.IsPrimary {
					position = 1
				}

				// Determine alt text based on media type
				altText := ""
				switch media.MediaType {
				case "PB": // Product Image (Produktbilde)
					altText = product.Title + " - Product Image"
				case "FDV": // Documentation (Forvaltning, Drift, Vedlikehold)
					altText = product.Title + " - Documentation"
				case "MTG": // Assembly/Mounting Image
					altText = product.Title + " - Mounting Instructions"
				case "MB": // Environment Image (Miljøbilde)
					altText = product.Title + " - Environment Image"
				case "TEG": // Technical Drawing
					altText = product.Title + " - Technical Drawing"
				default:
					altText = product.Title + " - " + media.MediaType
				}

				product.Images = append(product.Images, models.ProductImage{
					ID:        media.GUID,
					SourceURL: media.URL,
					Position:  position,
					Alt:       altText,
					Status:    "pending",
					Source:    "nobb",
				})
			}
		}
	}
	if len(item.Suppliers) > 0 {
		fieldsUpdated = append(fieldsUpdated, "suppliers")
	}

	// Check if images were added
	imagesAdded := false
	for _, img := range product.Images {
		if img.Source == "nobb" {
			imagesAdded = true
			break
		}
	}
	if imagesAdded {
		fieldsUpdated = append(fieldsUpdated, "images")
	}

	// Extract comprehensive package info from all suppliers
	for _, sup := range item.Suppliers {
		for _, pkg := range sup.Packages {
			product.PackageInfo = append(product.PackageInfo, models.PackageInfo{
				Type:            pkg.Class,
				GTIN:            pkg.GTIN,
				Weight:          pkg.Weight,
				WeightUnit:      "kg",
				Length:          float64(pkg.Length),
				Width:           float64(pkg.Width),
				Height:          float64(pkg.Height),
				DimUnit:         "mm",
				Volume:          pkg.Volume,
				IsPCU:           pkg.IsPCU,
				MinOrderQty:     pkg.MinOrderQty,
				Deliverable:     pkg.Deliverable,
				Stocked:         pkg.Stocked,
				CalculatedCount: pkg.CalculatedCount,
				ConsistsOfCount: pkg.ConsistsOfCount,
				ConsistsOfUnit:  pkg.ConsistsOfUnit,
				DangerousGoods:  pkg.DangerousGoods,
				DGUNNumber:      pkg.DGUNNumber,
			})
		}
	}
	if len(product.PackageInfo) > 0 {
		fieldsUpdated = append(fieldsUpdated, "package_info")
	}

	return fieldsUpdated
}
