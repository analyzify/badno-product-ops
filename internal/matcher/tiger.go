package matcher

import (
	"math/rand"
	"strings"
	"time"

	"github.com/badno/badops/pkg/models"
)

// TigerMatcher matches products against Tiger.nl catalog
type TigerMatcher struct {
	catalog   map[string]string
	skuMapper *SKUMapper
	scraper   *TigerScraper
}

// NewTigerMatcher creates a new Tiger.nl matcher
func NewTigerMatcher() *TigerMatcher {
	// Simulated Tiger.nl product catalog
	catalog := map[string]string{
		"boston":    "https://www.tiger.nl/nl/badkameraccessoires/boston-series",
		"melbourne": "https://www.tiger.nl/nl/badkameraccessoires/melbourne-series",
		"urban":     "https://www.tiger.nl/nl/badkameraccessoires/urban-series",
		"tokyo":     "https://www.tiger.nl/nl/badkameraccessoires/tokyo-series",
		"impuls":    "https://www.tiger.nl/nl/badkameraccessoires/impuls-series",
		"tune":      "https://www.tiger.nl/nl/badkameraccessoires/tune-series",
		"items":     "https://www.tiger.nl/nl/badkameraccessoires/items-series",
		"nomad":     "https://www.tiger.nl/nl/badkameraccessoires/nomad-series",
		"shelf":     "https://www.tiger.nl/nl/badkameraccessoires/planken",
		"toilet":    "https://www.tiger.nl/nl/badkameraccessoires/toiletaccessoires",
		"towel":     "https://www.tiger.nl/nl/badkameraccessoires/handdoekhouders",
		"soap":      "https://www.tiger.nl/nl/badkameraccessoires/zeepdispensers",
		"hook":      "https://www.tiger.nl/nl/badkameraccessoires/haken",
		"mirror":    "https://www.tiger.nl/nl/badkameraccessoires/spiegels",
		"shower":    "https://www.tiger.nl/nl/badkameraccessoires/doucheaccessoires",
	}

	return &TigerMatcher{
		catalog:   catalog,
		skuMapper: NewSKUMapper(),
		scraper:   NewTigerScraper(),
	}
}

// Match attempts to match a product against Tiger.nl
func (m *TigerMatcher) Match(product models.Product) (string, float64) {
	nameLower := strings.ToLower(product.Name)

	// Find matching keywords
	var matchedURL string
	matchCount := 0

	for keyword, url := range m.catalog {
		if strings.Contains(nameLower, keyword) {
			matchedURL = url
			matchCount++
		}
	}

	// Calculate confidence score with some randomness for demo
	rand.Seed(time.Now().UnixNano())
	baseScore := 0.0

	switch {
	case matchCount >= 2:
		baseScore = 0.90 + rand.Float64()*0.10 // 90-100%
	case matchCount == 1:
		baseScore = 0.75 + rand.Float64()*0.15 // 75-90%
	default:
		baseScore = 0.30 + rand.Float64()*0.30 // 30-60%
	}

	if matchedURL == "" {
		// Generate a plausible URL for demo
		matchedURL = "https://www.tiger.nl/nl/badkameraccessoires/producten"
	}

	return matchedURL, baseScore
}

// LookupBySKU attempts to find a product on Tiger.nl using the SKU
// Returns the product info, list of valid image URLs, and error if any
func (m *TigerMatcher) LookupBySKU(sku string, productName string) (*TigerProduct, error) {
	// Check cache first
	if cached, found := m.scraper.GetCached(sku); found {
		return cached, nil
	}

	// Get candidate Tiger.nl IDs
	candidateIDs := m.skuMapper.MapSKU(sku)

	// Get the appropriate base path
	basePath := m.skuMapper.GetCategoryPath(productName)

	// Get product types to try
	productTypes := m.skuMapper.GetProductTypes()

	// Try each candidate ID
	for _, tigerID := range candidateIDs {
		product, err := m.scraper.FindProductByID(tigerID, basePath, productTypes)
		if err == nil && product != nil {
			// Cache the successful result
			m.scraper.SetCached(sku, product)
			return product, nil
		}
	}

	// Cache the not-found result
	m.scraper.SetCached(sku, nil)
	return nil, nil
}

// GetSKUMapper returns the SKU mapper for direct access
func (m *TigerMatcher) GetSKUMapper() *SKUMapper {
	return m.skuMapper
}

// GetScraper returns the scraper for direct access
func (m *TigerMatcher) GetScraper() *TigerScraper {
	return m.scraper
}
