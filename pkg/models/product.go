package models

import "time"

// ProductStatus represents the current state of a product in the enhancement pipeline
type ProductStatus string

const (
	StatusPending    ProductStatus = "pending"
	StatusProcessing ProductStatus = "processing"
	StatusEnhanced   ProductStatus = "enhanced"
	StatusReview     ProductStatus = "review"
	StatusApproved   ProductStatus = "approved"
	StatusExported   ProductStatus = "exported"
	StatusFailed     ProductStatus = "failed"
)

// Product represents a product from the Matrixify export (legacy, for backward compatibility)
type Product struct {
	SKU            string   `json:"sku"`
	Name           string   `json:"name"`
	Brand          string   `json:"brand"`
	ExistingImages []string `json:"existing_images"`
	MatchedURL     string   `json:"matched_url,omitempty"`
	MatchScore     float64  `json:"match_score,omitempty"`
	NewImages      []Image  `json:"new_images,omitempty"`
}

// EnhancedProduct represents a fully enriched product with data from multiple sources
type EnhancedProduct struct {
	// Identity
	ID         string `json:"id,omitempty"`          // Internal ID
	SKU        string `json:"sku"`                   // Variant SKU (e.g., CO-T309012)
	Handle     string `json:"handle,omitempty"`      // Shopify handle
	Barcode    string `json:"barcode,omitempty"`     // EAN/UPC barcode
	NOBBNumber string `json:"nobb_number,omitempty"` // NOBB database number

	// Content
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	ProductType string   `json:"product_type,omitempty"`
	Tags        []string `json:"tags,omitempty"`

	// Pricing
	Price *Price `json:"price,omitempty"`

	// Physical Attributes
	Dimensions *Dimensions `json:"dimensions,omitempty"`
	Weight     *Weight     `json:"weight,omitempty"`

	// Media
	Images []ProductImage `json:"images,omitempty"`

	// Specifications (key-value pairs from various sources)
	Specifications map[string]string `json:"specifications,omitempty"`

	// Structured Properties (from NOBB)
	Properties []Property `json:"properties,omitempty"`

	// Supply Chain (from NOBB)
	Suppliers   []Supplier    `json:"suppliers,omitempty"`
	PackageInfo []PackageInfo `json:"package_info,omitempty"`

	// Enhancement Tracking
	Enhancements []Enhancement `json:"enhancements,omitempty"`
	Status       ProductStatus `json:"status"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Legacy fields for backward compatibility with existing state file
	LegacyMatchedURL string  `json:"matched_url,omitempty"`
	LegacyMatchScore float64 `json:"match_score,omitempty"`
}

// Price represents product pricing information
type Price struct {
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`      // NOK, EUR, USD
	CompareAt    float64 `json:"compare_at"`    // Original price for discounts
	CostPerItem  float64 `json:"cost_per_item"` // Wholesale cost
	TaxIncluded  bool    `json:"tax_included"`
	LastUpdated  time.Time `json:"last_updated,omitempty"`
}

// Dimensions represents physical dimensions
type Dimensions struct {
	Length float64 `json:"length"` // in millimeters
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Unit   string  `json:"unit"` // mm, cm, m
}

// Weight represents product weight
type Weight struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"` // g, kg
}

// ProductImage represents an image associated with a product
type ProductImage struct {
	ID          string    `json:"id,omitempty"`
	SourceURL   string    `json:"source_url"`
	LocalPath   string    `json:"local_path,omitempty"`
	Position    int       `json:"position"`
	Alt         string    `json:"alt,omitempty"`
	Width       int       `json:"width,omitempty"`
	Height      int       `json:"height,omitempty"`
	Status      string    `json:"status"` // pending, downloaded, resized, uploaded, failed
	Source      string    `json:"source"` // shopify, tiger_nl, nobb
	ResizedPaths map[string]string `json:"resized_paths,omitempty"`
	DownloadedAt time.Time `json:"downloaded_at,omitempty"`
}

// Property represents a structured property from NOBB or other sources
type Property struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Unit        string `json:"unit,omitempty"`
	Source      string `json:"source"` // nobb, tiger_nl, shopify
}

// Supplier represents supplier information from NOBB
type Supplier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	GLN         string `json:"gln,omitempty"`          // Global Location Number
	ArticleNo   string `json:"article_no,omitempty"`   // Supplier's article number
	IsPrimary   bool   `json:"is_primary"`
}

// PackageInfo represents packaging information from NOBB
type PackageInfo struct {
	Type        string  `json:"type"`         // PIECE, INNER, OUTER, PALLET
	Quantity    int     `json:"quantity"`
	GTIN        string  `json:"gtin,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
	WeightUnit  string  `json:"weight_unit,omitempty"`
	Length      float64 `json:"length,omitempty"`
	Width       float64 `json:"width,omitempty"`
	Height      float64 `json:"height,omitempty"`
	DimUnit     string  `json:"dim_unit,omitempty"`
}

// Enhancement represents a single enhancement action performed on a product
type Enhancement struct {
	Source      string    `json:"source"`      // nobb, tiger_nl, manual
	Action      string    `json:"action"`      // images_added, properties_added, etc.
	Details     string    `json:"details"`     // Human-readable description
	FieldsAdded []string  `json:"fields_added,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
}

// Image represents an image to be processed (legacy, for backward compatibility)
type Image struct {
	SourceURL    string            `json:"source_url"`
	OriginalPath string            `json:"original_path,omitempty"`
	ResizedPaths map[string]string `json:"resized_paths,omitempty"`
	Status       string            `json:"status"` // pending, downloaded, resized, failed
}

// Report represents the output report
type Report struct {
	TotalProducts   int       `json:"total_products"`
	MatchedProducts int       `json:"matched_products"`
	ImagesFound     int       `json:"images_found"`
	ImagesProcessed int       `json:"images_processed"`
	Products        []Product `json:"products"`
}

// ToEnhancedProduct converts a legacy Product to an EnhancedProduct
func (p *Product) ToEnhancedProduct() *EnhancedProduct {
	ep := &EnhancedProduct{
		SKU:              p.SKU,
		Title:            p.Name,
		Vendor:           p.Brand,
		Status:           StatusPending,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		LegacyMatchedURL: p.MatchedURL,
		LegacyMatchScore: p.MatchScore,
		Specifications:   make(map[string]string),
	}

	// Convert existing images
	for i, imgURL := range p.ExistingImages {
		ep.Images = append(ep.Images, ProductImage{
			SourceURL: imgURL,
			Position:  i + 1,
			Status:    "existing",
			Source:    "shopify",
		})
	}

	// Convert new images
	for _, img := range p.NewImages {
		ep.Images = append(ep.Images, ProductImage{
			SourceURL:    img.SourceURL,
			LocalPath:    img.OriginalPath,
			Status:       img.Status,
			Source:       "tiger_nl",
			ResizedPaths: img.ResizedPaths,
		})
	}

	return ep
}

// ToLegacyProduct converts an EnhancedProduct back to a legacy Product
func (ep *EnhancedProduct) ToLegacyProduct() *Product {
	p := &Product{
		SKU:        ep.SKU,
		Name:       ep.Title,
		Brand:      ep.Vendor,
		MatchedURL: ep.LegacyMatchedURL,
		MatchScore: ep.LegacyMatchScore,
	}

	// Extract existing images (from Shopify)
	for _, img := range ep.Images {
		if img.Source == "shopify" || img.Status == "existing" {
			p.ExistingImages = append(p.ExistingImages, img.SourceURL)
		}
	}

	// Extract new images (from Tiger.nl or other sources)
	for _, img := range ep.Images {
		if img.Source != "shopify" && img.Status != "existing" {
			p.NewImages = append(p.NewImages, Image{
				SourceURL:    img.SourceURL,
				OriginalPath: img.LocalPath,
				ResizedPaths: img.ResizedPaths,
				Status:       img.Status,
			})
		}
	}

	return p
}
