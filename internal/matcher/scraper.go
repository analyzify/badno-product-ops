package matcher

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TigerProduct represents a product found on Tiger.nl
type TigerProduct struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	ImageURLs []string `json:"image_urls"`
}

// CacheEntry stores a cached lookup result
type CacheEntry struct {
	SKU       string        `json:"sku"`
	Product   *TigerProduct `json:"product,omitempty"`
	NotFound  bool          `json:"not_found,omitempty"`
	CachedAt  time.Time     `json:"cached_at"`
}

// TigerScraper scrapes product data from Tiger.nl
type TigerScraper struct {
	client       *http.Client
	baseURL      string
	cache        map[string]*CacheEntry
	cacheMu      sync.RWMutex
	cacheFile    string
	rateLimit    time.Duration
	lastRequest  time.Time
	rateLimitMu  sync.Mutex
}

// NewTigerScraper creates a new Tiger.nl scraper with caching and rate limiting
func NewTigerScraper() *TigerScraper {
	s := &TigerScraper{
		client:    &http.Client{Timeout: 30 * time.Second},
		baseURL:   "https://tiger.nl",
		cache:     make(map[string]*CacheEntry),
		cacheFile: "output/.tiger-cache.json",
		rateLimit: 150 * time.Millisecond, // 150ms between requests
	}
	s.loadCache()
	return s
}

// loadCache loads the cache from disk
func (s *TigerScraper) loadCache() {
	data, err := os.ReadFile(s.cacheFile)
	if err != nil {
		return // No cache file yet
	}
	var entries []CacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	for _, e := range entries {
		entry := e // Copy to avoid pointer issues
		s.cache[e.SKU] = &entry
	}
}

// saveCache saves the cache to disk
func (s *TigerScraper) saveCache() {
	s.cacheMu.RLock()
	entries := make([]CacheEntry, 0, len(s.cache))
	for _, e := range s.cache {
		entries = append(entries, *e)
	}
	s.cacheMu.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll("output", 0755)
	os.WriteFile(s.cacheFile, data, 0644)
}

// GetCached returns a cached result if available
func (s *TigerScraper) GetCached(sku string) (*TigerProduct, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if entry, ok := s.cache[sku]; ok {
		// Cache entries valid for 24 hours
		if time.Since(entry.CachedAt) < 24*time.Hour {
			if entry.NotFound {
				return nil, true // Cached as not found
			}
			return entry.Product, true
		}
	}
	return nil, false
}

// SetCached stores a result in the cache
func (s *TigerScraper) SetCached(sku string, product *TigerProduct) {
	s.cacheMu.Lock()
	s.cache[sku] = &CacheEntry{
		SKU:      sku,
		Product:  product,
		NotFound: product == nil,
		CachedAt: time.Now(),
	}
	s.cacheMu.Unlock()
	s.saveCache() // Save synchronously to ensure it completes
}

// rateLimitWait waits if needed to respect rate limiting
func (s *TigerScraper) rateLimitWait() {
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()

	elapsed := time.Since(s.lastRequest)
	if elapsed < s.rateLimit {
		time.Sleep(s.rateLimit - elapsed)
	}
	s.lastRequest = time.Now()
}

// doGet performs a rate-limited GET request
func (s *TigerScraper) doGet(url string) (*http.Response, error) {
	s.rateLimitWait()
	return s.client.Get(url)
}

// doHead performs a rate-limited HEAD request
func (s *TigerScraper) doHead(url string) (*http.Response, error) {
	s.rateLimitWait()
	return s.client.Head(url)
}

// FindProduct searches for a product on Tiger.nl and returns its images
func (s *TigerScraper) FindProduct(productName string) (*TigerProduct, error) {
	// Map common product types to Tiger.nl category URLs
	searchURL := s.buildSearchURL(productName)

	resp, err := s.doGet(searchURL)
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
	resp, err := s.doGet(productURL)
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

// FindProductByID finds a product on Tiger.nl using a direct product ID
// This is more reliable than name-based searching as it goes directly to the product page
func (s *TigerScraper) FindProductByID(tigerID string, basePath string, productTypes []string) (*TigerProduct, error) {
	// Try each product type until we find a valid page
	for _, productType := range productTypes {
		// First, try to find the exact URL by fetching the category page
		// and looking for our product ID
		categoryURL := fmt.Sprintf("%s%s%s/", s.baseURL, basePath, productType)
		resp, err := s.doGet(categoryURL)
		if err != nil {
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			continue
		}

		html := string(body)

		// Look for our product ID in the category page
		// Pattern: /basePath/productType/tigerID-slug/
		pattern := fmt.Sprintf(`%s%s/%s-[^"'/]+/`, basePath, productType, tigerID)
		re := regexp.MustCompile(pattern)
		match := re.FindString(html)

		if match != "" {
			productURL := s.baseURL + match
			images, err := s.scrapeProductImagesWithValidation(productURL)
			if err != nil {
				continue
			}

			return &TigerProduct{
				Name:      tigerID,
				URL:       productURL,
				ImageURLs: images,
			}, nil
		}
	}

	return nil, fmt.Errorf("product not found for ID: %s", tigerID)
}

// FindProductByIDDirect tries to directly access product pages by constructing URLs
// This is faster than searching category pages
func (s *TigerScraper) FindProductByIDDirect(tigerID string, basePath string, productTypes []string) (*TigerProduct, error) {
	// Build candidate URLs and try each one
	for _, productType := range productTypes {
		// Try common slug patterns
		slugPatterns := []string{
			tigerID + "-" + productType,
			tigerID + "-" + strings.ReplaceAll(productType, "-", " "),
		}

		for _, slug := range slugPatterns {
			productURL := fmt.Sprintf("%s%s%s/%s/", s.baseURL, basePath, productType, slug)

			resp, err := s.doHead(productURL)
			if err != nil {
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == 200 {
				images, err := s.scrapeProductImagesWithValidation(productURL)
				if err != nil {
					continue
				}

				return &TigerProduct{
					Name:      tigerID,
					URL:       productURL,
					ImageURLs: images,
				}, nil
			}
		}
	}

	// Fall back to searching category pages
	return s.FindProductByID(tigerID, basePath, productTypes)
}

// scrapeProductImagesWithValidation extracts and validates image URLs from a product page
func (s *TigerScraper) scrapeProductImagesWithValidation(productURL string) ([]string, error) {
	resp, err := s.doGet(productURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("page returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
	var validImages []string

	// Extract PIM image URLs (product images)
	pimRe := regexp.MustCompile(`/pim/528_[a-f0-9-]+`)
	pimMatches := pimRe.FindAllString(html, -1)

	seen := make(map[string]bool)
	for _, match := range pimMatches {
		if !seen[match] {
			seen[match] = true
			fullURL := fmt.Sprintf("%s%s?width=1200&height=1200&format=jpg&quality=90", s.baseURL, match)

			// Validate the image URL
			if s.ValidateImageURL(fullURL) {
				validImages = append(validImages, fullURL)
			}
		}
	}

	return validImages, nil
}

// ValidateImageURL checks if an image URL is accessible (returns 200)
func (s *TigerScraper) ValidateImageURL(imageURL string) bool {
	resp, err := s.doHead(imageURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

// GetImageCount returns the count of valid images for a product
func (s *TigerScraper) GetImageCount(productURL string) (int, error) {
	images, err := s.scrapeProductImagesWithValidation(productURL)
	if err != nil {
		return 0, err
	}
	return len(images), nil
}
