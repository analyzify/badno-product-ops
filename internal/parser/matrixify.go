package parser

import (
	"encoding/csv"
	"os"
	"strings"

	"github.com/badno/badops/pkg/models"
)

// ParseMatrixifyCSV parses a Matrixify export CSV file
func ParseMatrixifyCSV(filepath string) ([]models.Product, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return []models.Product{}, nil
	}

	// Find column indices
	header := records[0]
	handleIdx := findColumn(header, "Handle")
	titleIdx := findColumn(header, "Title")
	vendorIdx := findColumn(header, "Vendor")
	imageIdx := findColumn(header, "Image Src")

	var products []models.Product
	for _, row := range records[1:] {
		if len(row) <= handleIdx || len(row) <= titleIdx {
			continue
		}

		// Check if it's a Tiger product
		vendor := ""
		if vendorIdx >= 0 && len(row) > vendorIdx {
			vendor = row[vendorIdx]
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
			SKU:            row[handleIdx],
			Name:           row[titleIdx],
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
