package matcher

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// TigerProduct represents a product found on Tiger.nl
type TigerProduct struct {
	Name      string
	URL       string
	ImageURLs []string
}

// TigerScraper scrapes product data from Tiger.nl
type TigerScraper struct {
	client  *http.Client
	baseURL string
}

// NewTigerScraper creates a new Tiger.nl scraper
func NewTigerScraper() *TigerScraper {
	return &TigerScraper{
		client:  &http.Client{},
		baseURL: "https://tiger.nl",
	}
}

// FindProduct searches for a product on Tiger.nl and returns its images
func (s *TigerScraper) FindProduct(productName string) (*TigerProduct, error) {
	// Map common product types to Tiger.nl category URLs
	searchURL := s.buildSearchURL(productName)

	resp, err := s.client.Get(searchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	// Extract product page URL from search results
	productURL := s.extractProductURL(html, productName)
	if productURL == "" {
		return nil, fmt.Errorf("product not found: %s", productName)
	}

	// Fetch product page and extract images
	images, err := s.scrapeProductImages(productURL)
	if err != nil {
		return nil, err
	}

	return &TigerProduct{
		Name:      productName,
		URL:       productURL,
		ImageURLs: images,
	}, nil
}

// buildSearchURL constructs a search URL based on product name keywords
func (s *TigerScraper) buildSearchURL(productName string) string {
	nameLower := strings.ToLower(productName)

	// Map product types to Tiger.nl categories
	categoryMap := map[string]string{
		"toalettrullholder": "toiletrolhouder",
		"toilet roll":       "toiletrolhouder",
		"toalettbørste":     "toiletborstel",
		"toilet brush":      "toiletborstel",
		"håndklestang":      "handdoekhouder",
		"towel rail":        "handdoekhouder",
		"towel bar":         "handdoekhouder",
		"krok":              "haak",
		"hook":              "haak",
		"dusjkurv":          "douchekorf",
		"shower caddy":      "douchekorf",
		"speil":             "spiegel",
		"mirror":            "spiegel",
	}

	// Map series names
	seriesMap := map[string]string{
		"boston":  "productserie-boston",
		"urban":   "productserie-urban",
		"2-store": "productserie-2-store",
		"carv":    "productserie-carv",
		"tune":    "productserie-tune",
		"impuls":  "productserie-impuls",
		"nomad":   "productserie-nomad",
		"items":   "productserie-items",
	}

	// Find matching category
	var category string
	for keyword, cat := range categoryMap {
		if strings.Contains(nameLower, keyword) {
			category = cat
			break
		}
	}

	// Find matching series
	var series string
	for keyword, ser := range seriesMap {
		if strings.Contains(nameLower, keyword) {
			series = ser
			break
		}
	}

	// Build URL with filters
	url := fmt.Sprintf("%s/producten/badkameraccessoires/", s.baseURL)
	if series != "" || category != "" {
		url += "?"
		if series != "" {
			url += "productserie=" + series
		}
		if category != "" {
			if series != "" {
				url += "&"
			}
			url += "category=" + category
		}
	}

	return url
}

// extractProductURL finds the product detail page URL from search results
func (s *TigerScraper) extractProductURL(html, productName string) string {
	// Look for product links in the HTML
	// Pattern: /producten/badkameraccessoires/[type]/[id]-[name]/
	re := regexp.MustCompile(`/producten/badkameraccessoires/[^"]+/\d+-[^"]+/`)
	matches := re.FindAllString(html, -1)

	if len(matches) > 0 {
		return s.baseURL + matches[0]
	}
	return ""
}

// scrapeProductImages extracts all image URLs from a product page
func (s *TigerScraper) scrapeProductImages(productURL string) ([]string, error) {
	resp, err := s.client.Get(productURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
	var images []string

	// Extract PIM image URLs (product images)
	// Pattern: /pim/528_[uuid]
	pimRe := regexp.MustCompile(`/pim/528_[a-f0-9-]+`)
	pimMatches := pimRe.FindAllString(html, -1)

	seen := make(map[string]bool)
	for _, match := range pimMatches {
		if !seen[match] {
			seen[match] = true
			// Request high-res version
			fullURL := fmt.Sprintf("%s%s?width=1200&height=1200&format=jpg&quality=90", s.baseURL, match)
			images = append(images, fullURL)
		}
	}

	// Also extract media images (lifestyle shots)
	// Pattern: /media/[hash]/[filename]
	mediaRe := regexp.MustCompile(`/media/[a-z0-9]+/[^"'\s]+\.(jpg|jpeg|png|webp)`)
	mediaMatches := mediaRe.FindAllString(html, -1)

	for _, match := range mediaMatches {
		if !seen[match] && !strings.Contains(match, "icon") && !strings.Contains(match, "logo") {
			seen[match] = true
			images = append(images, s.baseURL+match)
		}
	}

	return images, nil
}

// GetProductImages is a convenience method that returns image count and URLs
func (s *TigerScraper) GetProductImages(productName string) (int, []string, error) {
	product, err := s.FindProduct(productName)
	if err != nil {
		return 0, nil, err
	}
	return len(product.ImageURLs), product.ImageURLs, nil
}
