package file

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/badno/badops/internal/output"
	"github.com/badno/badops/pkg/models"
)

const CSVAdapterName = "csv"

// CSVConfig holds CSV file output configuration
type CSVConfig struct {
	OutputDir string // Directory for output files
}

// CSVAdapter implements the output.Adapter interface for CSV files
type CSVAdapter struct {
	*output.BaseAdapter
	config CSVConfig
}

// NewCSVAdapter creates a new CSV file adapter
func NewCSVAdapter(cfg CSVConfig) *CSVAdapter {
	if cfg.OutputDir == "" {
		cfg.OutputDir = "output"
	}

	return &CSVAdapter{
		BaseAdapter: output.NewBaseAdapter(
			CSVAdapterName,
			[]output.Format{output.FormatMatrixify, output.FormatShopify},
		),
		config: cfg,
	}
}

// Connect creates the output directory
func (a *CSVAdapter) Connect(ctx context.Context) error {
	if err := os.MkdirAll(a.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	a.SetConnected(true)
	return nil
}

// Close cleans up resources
func (a *CSVAdapter) Close() error {
	a.SetConnected(false)
	return nil
}

// Test verifies the output directory is writable
func (a *CSVAdapter) Test(ctx context.Context) error {
	testFile := filepath.Join(a.config.OutputDir, ".test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("output directory not writable: %w", err)
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

// ExportProducts exports products to a CSV file
func (a *CSVAdapter) ExportProducts(ctx context.Context, products []models.EnhancedProduct, opts output.ExportOptions) (*output.ExportResult, error) {
	result := &output.ExportResult{
		StartedAt: time.Now(),
	}

	if !a.IsConnected() {
		if err := a.Connect(ctx); err != nil {
			result.Error = err
			return result, err
		}
	}

	// Filter products if needed
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
		result.Details = fmt.Sprintf("Dry run: would export %d products", len(filteredProducts))
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Determine filename
	filename := opts.OutputPath
	if filename == "" {
		timestamp := time.Now().Format("2006-01-02_150405")
		filename = filepath.Join(a.config.OutputDir, fmt.Sprintf("products_%s.csv", timestamp))
	}

	// Create file
	f, err := os.Create(filename)
	if err != nil {
		result.Error = err
		return result, err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// Write based on format
	var imagesExported int
	switch opts.Format {
	case output.FormatMatrixify:
		imagesExported, err = a.writeMatrixifyFormat(writer, filteredProducts, opts)
	default:
		imagesExported, err = a.writeShopifyFormat(writer, filteredProducts, opts)
	}

	if err != nil {
		result.Error = err
		return result, err
	}

	result.Destination = filename
	result.ProductsExported = len(filteredProducts)
	result.ImagesExported = imagesExported
	result.Success = true
	result.Details = fmt.Sprintf("Exported %d products to %s", len(filteredProducts), filename)
	result.CompletedAt = time.Now()

	return result, nil
}

// writeMatrixifyFormat writes products in Matrixify-compatible format
func (a *CSVAdapter) writeMatrixifyFormat(w *csv.Writer, products []models.EnhancedProduct, opts output.ExportOptions) (int, error) {
	// Matrixify headers
	headers := []string{
		"Handle",
		"Title",
		"Body (HTML)",
		"Vendor",
		"Product Category",
		"Type",
		"Tags",
		"Published",
		"Option1 Name",
		"Option1 Value",
		"Variant SKU",
		"Variant Grams",
		"Variant Inventory Tracker",
		"Variant Inventory Qty",
		"Variant Inventory Policy",
		"Variant Fulfillment Service",
		"Variant Price",
		"Variant Compare At Price",
		"Variant Requires Shipping",
		"Variant Taxable",
		"Variant Barcode",
		"Image Src",
		"Image Position",
		"Image Alt Text",
		"SEO Title",
		"SEO Description",
		"Variant Weight Unit",
	}

	if err := w.Write(headers); err != nil {
		return 0, err
	}

	imagesExported := 0

	for _, p := range products {
		// Create handle from title if not set
		handle := p.Handle
		if handle == "" {
			handle = strings.ToLower(strings.ReplaceAll(p.Title, " ", "-"))
		}

		// Base product row
		row := make([]string, len(headers))
		row[0] = handle                              // Handle
		row[1] = p.Title                             // Title
		row[2] = p.Description                       // Body (HTML)
		row[3] = p.Vendor                            // Vendor
		row[4] = ""                                  // Product Category
		row[5] = p.ProductType                       // Type
		row[6] = strings.Join(p.Tags, ", ")          // Tags
		row[7] = "TRUE"                              // Published
		row[8] = "Title"                             // Option1 Name
		row[9] = "Default Title"                     // Option1 Value
		row[10] = p.SKU                              // Variant SKU

		// Weight
		if p.Weight != nil {
			grams := p.Weight.Value
			if p.Weight.Unit == "kg" {
				grams = p.Weight.Value * 1000
			}
			row[11] = fmt.Sprintf("%.0f", grams)     // Variant Grams
		}

		row[12] = "shopify"                          // Variant Inventory Tracker
		row[13] = ""                                 // Variant Inventory Qty
		row[14] = "deny"                             // Variant Inventory Policy
		row[15] = "manual"                           // Variant Fulfillment Service

		// Price
		if p.Price != nil {
			row[16] = fmt.Sprintf("%.2f", p.Price.Amount)     // Variant Price
			if p.Price.CompareAt > 0 {
				row[17] = fmt.Sprintf("%.2f", p.Price.CompareAt) // Variant Compare At Price
			}
		}

		row[18] = "TRUE"                             // Variant Requires Shipping
		row[19] = "TRUE"                             // Variant Taxable
		row[20] = p.Barcode                          // Variant Barcode

		// First image in main row
		if len(p.Images) > 0 && opts.IncludeImages {
			row[21] = p.Images[0].SourceURL          // Image Src
			row[22] = "1"                            // Image Position
			row[23] = p.Images[0].Alt                // Image Alt Text
			imagesExported++
		}

		row[24] = ""                                 // SEO Title
		row[25] = ""                                 // SEO Description

		if p.Weight != nil {
			row[26] = "g"                            // Variant Weight Unit
		}

		if err := w.Write(row); err != nil {
			return imagesExported, err
		}

		// Additional image rows (Matrixify format uses separate rows for each image)
		if opts.IncludeImages && len(p.Images) > 1 {
			for i := 1; i < len(p.Images); i++ {
				img := p.Images[i]
				imgRow := make([]string, len(headers))
				imgRow[0] = handle                   // Handle (same as product)
				imgRow[21] = img.SourceURL           // Image Src
				imgRow[22] = fmt.Sprintf("%d", i+1)  // Image Position
				imgRow[23] = img.Alt                 // Image Alt Text

				if err := w.Write(imgRow); err != nil {
					return imagesExported, err
				}
				imagesExported++
			}
		}
	}

	return imagesExported, nil
}

// writeShopifyFormat writes products in standard Shopify CSV format
func (a *CSVAdapter) writeShopifyFormat(w *csv.Writer, products []models.EnhancedProduct, opts output.ExportOptions) (int, error) {
	// Simplified Shopify headers
	headers := []string{
		"Handle",
		"Title",
		"Body (HTML)",
		"Vendor",
		"Type",
		"Tags",
		"Variant SKU",
		"Variant Price",
		"Variant Barcode",
		"Image Src",
	}

	if err := w.Write(headers); err != nil {
		return 0, err
	}

	imagesExported := 0

	for _, p := range products {
		handle := p.Handle
		if handle == "" {
			handle = strings.ToLower(strings.ReplaceAll(p.Title, " ", "-"))
		}

		row := []string{
			handle,
			p.Title,
			p.Description,
			p.Vendor,
			p.ProductType,
			strings.Join(p.Tags, ", "),
			p.SKU,
			"",
			p.Barcode,
			"",
		}

		if p.Price != nil {
			row[7] = fmt.Sprintf("%.2f", p.Price.Amount)
		}

		// Combine all image URLs
		if opts.IncludeImages && len(p.Images) > 0 {
			var urls []string
			for _, img := range p.Images {
				urls = append(urls, img.SourceURL)
				imagesExported++
			}
			row[9] = strings.Join(urls, ";")
		}

		if err := w.Write(row); err != nil {
			return imagesExported, err
		}
	}

	return imagesExported, nil
}
