package prices

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/badno/badops/internal/database"
	"github.com/google/uuid"
)

// CSVRecord represents a single row from the Reprice CSV export
type CSVRecord struct {
	SKU              string
	Barcode          string
	ProductTitle     string
	Vendor           string
	OwnPrice         float64
	OwnStock         bool
	OwnStockQuantity *int
	CompetitorName   string
	CompetitorPrice  float64
	CompetitorStock  bool
	CompetitorURL    string
	ObservedAt       time.Time
}

// ParseResult contains the results of parsing a Reprice CSV file
type ParseResult struct {
	Records         []CSVRecord
	Competitors     map[string]bool
	ProductCount    int
	ObservationTime time.Time
	Errors          []string
}

// Parser handles parsing of Reprice CSV files
type Parser struct {
	// Column indices (will be detected from header)
	colSKU              int
	colBarcode          int
	colTitle            int
	colVendor           int
	colOwnPrice         int
	colOwnStock         int
	colCompetitorPrefix string
}

// NewParser creates a new Reprice CSV parser
func NewParser() *Parser {
	return &Parser{
		colSKU:              -1,
		colBarcode:          -1,
		colTitle:            -1,
		colVendor:           -1,
		colOwnPrice:         -1,
		colOwnStock:         -1,
		colCompetitorPrefix: "competitor_",
	}
}

// ParseFile parses a Reprice CSV file and returns the records
func (p *Parser) ParseFile(filePath string) (*ParseResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	return p.Parse(file)
}

// Parse parses Reprice CSV data from a reader
func (p *Parser) Parse(r io.Reader) (*ParseResult, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // Allow variable number of fields
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Map header columns
	competitorCols := p.mapHeader(header)

	result := &ParseResult{
		Records:         make([]CSVRecord, 0),
		Competitors:     make(map[string]bool),
		ObservationTime: time.Now(),
	}

	// Track unique products
	seenProducts := make(map[string]bool)
	lineNum := 1

	// Read data rows
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: %v", lineNum, err))
			lineNum++
			continue
		}
		lineNum++

		// Skip empty rows
		if len(row) == 0 || (len(row) == 1 && row[0] == "") {
			continue
		}

		// Get base product info
		sku := p.getField(row, p.colSKU)
		if sku == "" {
			continue // Skip rows without SKU
		}

		barcode := p.getField(row, p.colBarcode)
		title := p.getField(row, p.colTitle)
		vendor := p.getField(row, p.colVendor)
		ownPrice := p.parseFloat(p.getField(row, p.colOwnPrice))
		ownStock := p.parseBool(p.getField(row, p.colOwnStock))

		// Track unique products
		seenProducts[sku] = true

		// Parse competitor data
		for competitorName, cols := range competitorCols {
			priceStr := p.getField(row, cols.priceCol)
			if priceStr == "" || priceStr == "-" || priceStr == "N/A" {
				continue // No price data for this competitor
			}

			price := p.parseFloat(priceStr)
			if price <= 0 {
				continue
			}

			stock := true
			if cols.stockCol >= 0 {
				stock = p.parseBool(p.getField(row, cols.stockCol))
			}

			url := ""
			if cols.urlCol >= 0 {
				url = p.getField(row, cols.urlCol)
			}

			record := CSVRecord{
				SKU:             sku,
				Barcode:         barcode,
				ProductTitle:    title,
				Vendor:          vendor,
				OwnPrice:        ownPrice,
				OwnStock:        ownStock,
				CompetitorName:  competitorName,
				CompetitorPrice: price,
				CompetitorStock: stock,
				CompetitorURL:   url,
				ObservedAt:      result.ObservationTime,
			}

			result.Records = append(result.Records, record)
			result.Competitors[competitorName] = true
		}
	}

	result.ProductCount = len(seenProducts)
	return result, nil
}

type competitorColumns struct {
	priceCol int
	stockCol int
	urlCol   int
}

// mapHeader maps column indices from the header row
func (p *Parser) mapHeader(header []string) map[string]competitorColumns {
	competitors := make(map[string]competitorColumns)

	for i, col := range header {
		colLower := strings.ToLower(strings.TrimSpace(col))

		switch {
		case colLower == "sku" || colLower == "variant sku" || colLower == "product_sku":
			p.colSKU = i
		case colLower == "barcode" || colLower == "ean" || colLower == "variant barcode":
			p.colBarcode = i
		case colLower == "title" || colLower == "product title" || colLower == "name" || colLower == "product_title":
			p.colTitle = i
		case colLower == "vendor" || colLower == "brand":
			p.colVendor = i
		case colLower == "price" || colLower == "our_price" || colLower == "own_price" || colLower == "variant price":
			p.colOwnPrice = i
		case colLower == "stock" || colLower == "in_stock" || colLower == "our_stock":
			p.colOwnStock = i
		default:
			// Check for competitor columns
			// Format: "Competitor Name" or "competitor_name_price" etc.
			competitorName := p.extractCompetitorName(col)
			if competitorName != "" {
				cols, exists := competitors[competitorName]
				if !exists {
					cols = competitorColumns{priceCol: -1, stockCol: -1, urlCol: -1}
				}

				if strings.Contains(colLower, "price") {
					cols.priceCol = i
				} else if strings.Contains(colLower, "stock") || strings.Contains(colLower, "availability") {
					cols.stockCol = i
				} else if strings.Contains(colLower, "url") || strings.Contains(colLower, "link") {
					cols.urlCol = i
				} else {
					// Assume it's a price column if no suffix
					cols.priceCol = i
				}

				competitors[competitorName] = cols
			}
		}
	}

	return competitors
}

// extractCompetitorName extracts the competitor name from a column header
func (p *Parser) extractCompetitorName(col string) string {
	col = strings.TrimSpace(col)
	colLower := strings.ToLower(col)

	// Skip standard columns
	standardCols := []string{"sku", "barcode", "ean", "title", "name", "vendor", "brand", "price", "stock", "id", "handle"}
	for _, std := range standardCols {
		if colLower == std {
			return ""
		}
	}

	// Remove common suffixes
	suffixes := []string{"_price", "_stock", "_url", "_link", " price", " stock", " url", " link", " availability"}
	name := col
	for _, suffix := range suffixes {
		if strings.HasSuffix(strings.ToLower(name), suffix) {
			name = name[:len(name)-len(suffix)]
			break
		}
	}

	// Clean up the name
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	return name
}

// getField safely gets a field from a row
func (p *Parser) getField(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

// parseFloat parses a string to float64
func (p *Parser) parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimPrefix(s, "kr")
	s = strings.TrimPrefix(s, "NOK")
	s = strings.TrimPrefix(s, "$")
	s = strings.TrimPrefix(s, "€")
	s = strings.TrimSpace(s)

	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// parseBool parses a string to bool
func (p *Parser) parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "yes" || s == "1" || s == "in stock" || s == "available"
}

// ConvertToPriceObservations converts parsed records to database price observations
func ConvertToPriceObservations(records []CSVRecord, productMap map[string]uuid.UUID, competitorMap map[string]int) []*database.PriceObservation {
	observations := make([]*database.PriceObservation, 0, len(records))

	for _, rec := range records {
		productID, ok := productMap[rec.SKU]
		if !ok {
			// Try barcode lookup
			if rec.Barcode != "" {
				productID, ok = productMap[rec.Barcode]
			}
			if !ok {
				continue // Product not found
			}
		}

		competitorID, ok := competitorMap[rec.CompetitorName]
		if !ok {
			continue // Competitor not found
		}

		obs := &database.PriceObservation{
			ProductID:    productID,
			CompetitorID: competitorID,
			Price:        rec.CompetitorPrice,
			Currency:     "NOK",
			InStock:      rec.CompetitorStock,
			ObservedAt:   rec.ObservedAt,
			Source:       "reprice_csv",
		}

		observations = append(observations, obs)
	}

	return observations
}

// ConvertToCompetitorProducts creates competitor product links from records
func ConvertToCompetitorProducts(records []CSVRecord, productMap map[string]uuid.UUID, competitorMap map[string]int) []*database.CompetitorProduct {
	// Use map to deduplicate
	links := make(map[string]*database.CompetitorProduct)

	for _, rec := range records {
		productID, ok := productMap[rec.SKU]
		if !ok {
			if rec.Barcode != "" {
				productID, ok = productMap[rec.Barcode]
			}
			if !ok {
				continue
			}
		}

		competitorID, ok := competitorMap[rec.CompetitorName]
		if !ok {
			continue
		}

		key := fmt.Sprintf("%s-%d", productID.String(), competitorID)
		if _, exists := links[key]; !exists {
			links[key] = &database.CompetitorProduct{
				ProductID:       productID,
				CompetitorID:    competitorID,
				URL:             rec.CompetitorURL,
				CompetitorTitle: rec.ProductTitle,
				IsActive:        true,
				MatchMethod:     "csv_import",
				MatchConfidence: 1.0,
			}
		}
	}

	result := make([]*database.CompetitorProduct, 0, len(links))
	for _, link := range links {
		result = append(result, link)
	}

	return result
}
