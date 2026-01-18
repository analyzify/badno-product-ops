package parser

import (
	"encoding/csv"
	"os"
	"strings"

	"github.com/badno/badops/pkg/models"
)

// ParseMatrixifyCSV parses a Matrixify or Shopify export CSV file
func ParseMatrixifyCSV(filepath string) ([]models.Product, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true // Handle Shopify's sometimes malformed CSV

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return []models.Product{}, nil
	}

	// Find column indices - support both Matrixify and Shopify formats
	header := records[0]
	// Clean BOM from first column if present
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}

	handleIdx := findColumn(header, "Handle")
	titleIdx := findColumn(header, "Title")
	vendorIdx := findColumn(header, "Vendor")
	imageIdx := findColumn(header, "Image Src")

	// Shopify-specific columns
	skuIdx := findColumn(header, "Variant SKU")
	if skuIdx < 0 {
		skuIdx = handleIdx // Fallback to Handle if no Variant SKU
	}

	// Track seen SKUs to avoid duplicates (Shopify has multiple rows per variant)
	seen := make(map[string]bool)
	var products []models.Product

	for _, row := range records[1:] {
		if len(row) <= titleIdx {
			continue
		}

		// Get SKU
		sku := ""
		if skuIdx >= 0 && len(row) > skuIdx {
			sku = strings.TrimSpace(row[skuIdx])
		}
		if sku == "" {
			continue
		}

		// Skip duplicates
		if seen[sku] {
			continue
		}
		seen[sku] = true

		// Check if it's a Tiger product
		vendor := ""
		if vendorIdx >= 0 && len(row) > vendorIdx {
			vendor = strings.TrimSpace(row[vendorIdx])
		}

		// Only include Tiger products
		if !strings.EqualFold(vendor, "Tiger") {
			continue
		}

		// Get title
		title := ""
		if titleIdx >= 0 && len(row) > titleIdx {
			title = strings.TrimSpace(row[titleIdx])
		}

		// Get images
		var images []string
		if imageIdx >= 0 && len(row) > imageIdx && row[imageIdx] != "" {
			images = strings.Split(row[imageIdx], ",")
			for i := range images {
				images[i] = strings.TrimSpace(images[i])
			}
		}

		product := models.Product{
			SKU:            sku,
			Name:           title,
			Brand:          vendor,
			ExistingImages: images,
		}
		products = append(products, product)
	}

	return products, nil
}

func findColumn(header []string, name string) int {
	for i, col := range header {
		if strings.EqualFold(strings.TrimSpace(col), name) {
			return i
		}
	}
	return -1
}
