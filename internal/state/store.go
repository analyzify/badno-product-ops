package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/badno/badops/pkg/models"
)

const (
	StateVersion = "2.0"
	DefaultStateFile = "output/.badops-state.json"
)

// HistoryEntry represents a single action in the history
type HistoryEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Action      string    `json:"action"`      // import, enhance, export, etc.
	Source      string    `json:"source"`      // shopify, nobb, tiger_nl, etc.
	Count       int       `json:"count"`       // Number of products affected
	Details     string    `json:"details"`     // Human-readable description
}

// StateFile represents the v2 state file structure
type StateFile struct {
	Version     string                            `json:"version"`
	Products    map[string]*models.EnhancedProduct `json:"products"` // Keyed by SKU
	History     []HistoryEntry                    `json:"history"`
	LastUpdated time.Time                         `json:"last_updated"`
}

// Store manages product state persistence
type Store struct {
	mu        sync.RWMutex
	filePath  string
	state     *StateFile
}

// NewStore creates a new state store
func NewStore(filePath string) *Store {
	if filePath == "" {
		filePath = DefaultStateFile
	}

	return &Store{
		filePath: filePath,
		state: &StateFile{
			Version:  StateVersion,
			Products: make(map[string]*models.EnhancedProduct),
			History:  []HistoryEntry{},
		},
	}
}

// Load reads the state from disk
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Initialize empty state
			s.state = &StateFile{
				Version:  StateVersion,
				Products: make(map[string]*models.EnhancedProduct),
				History:  []HistoryEntry{},
			}
			return nil
		}
		return err
	}

	// Try to detect version
	var versionCheck struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &versionCheck); err == nil && versionCheck.Version != "" {
		// v2 format
		var state StateFile
		if err := json.Unmarshal(data, &state); err != nil {
			return fmt.Errorf("failed to parse state file: %w", err)
		}
		s.state = &state
	} else {
		// v1 format (array of legacy products)
		return s.migrateFromV1(data)
	}

	return nil
}

// migrateFromV1 converts v1 state file to v2 format
func (s *Store) migrateFromV1(data []byte) error {
	var legacyProducts []models.Product
	if err := json.Unmarshal(data, &legacyProducts); err != nil {
		return fmt.Errorf("failed to parse legacy state file: %w", err)
	}

	s.state = &StateFile{
		Version:     StateVersion,
		Products:    make(map[string]*models.EnhancedProduct),
		History:     []HistoryEntry{},
		LastUpdated: time.Now(),
	}

	// Convert each legacy product
	for _, lp := range legacyProducts {
		ep := lp.ToEnhancedProduct()
		s.state.Products[ep.SKU] = ep
	}

	// Record migration in history
	s.state.History = append(s.state.History, HistoryEntry{
		Timestamp: time.Now(),
		Action:    "migrate",
		Source:    "v1",
		Count:     len(legacyProducts),
		Details:   fmt.Sprintf("Migrated %d products from v1 state format", len(legacyProducts)),
	})

	// Save the migrated state
	return s.saveInternal()
}

// Save writes the state to disk
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveInternal()
}

// saveInternal saves without acquiring lock (for internal use)
func (s *Store) saveInternal() error {
	s.state.LastUpdated = time.Now()

	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// GetProduct retrieves a product by SKU
func (s *Store) GetProduct(sku string) (*models.EnhancedProduct, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, exists := s.state.Products[sku]
	return p, exists
}

// SetProduct stores or updates a product
func (s *Store) SetProduct(product *models.EnhancedProduct) {
	s.mu.Lock()
	defer s.mu.Unlock()

	product.UpdatedAt = time.Now()
	s.state.Products[product.SKU] = product
}

// GetAllProducts returns all products
func (s *Store) GetAllProducts() []*models.EnhancedProduct {
	s.mu.RLock()
	defer s.mu.RUnlock()

	products := make([]*models.EnhancedProduct, 0, len(s.state.Products))
	for _, p := range s.state.Products {
		products = append(products, p)
	}
	return products
}

// GetProductsBySKUs returns products matching the given SKUs
func (s *Store) GetProductsBySKUs(skus []string) []*models.EnhancedProduct {
	s.mu.RLock()
	defer s.mu.RUnlock()

	products := make([]*models.EnhancedProduct, 0, len(skus))
	for _, sku := range skus {
		if p, exists := s.state.Products[sku]; exists {
			products = append(products, p)
		}
	}
	return products
}

// GetProductsByStatus returns products with a specific status
func (s *Store) GetProductsByStatus(status models.ProductStatus) []*models.EnhancedProduct {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var products []*models.EnhancedProduct
	for _, p := range s.state.Products {
		if p.Status == status {
			products = append(products, p)
		}
	}
	return products
}

// GetProductsByVendor returns products from a specific vendor
func (s *Store) GetProductsByVendor(vendor string) []*models.EnhancedProduct {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var products []*models.EnhancedProduct
	for _, p := range s.state.Products {
		if p.Vendor == vendor {
			products = append(products, p)
		}
	}
	return products
}

// Count returns the number of products
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.state.Products)
}

// Clear removes all products
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Products = make(map[string]*models.EnhancedProduct)
}

// AddHistory adds an entry to the history
func (s *Store) AddHistory(action, source string, count int, details string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.History = append(s.state.History, HistoryEntry{
		Timestamp: time.Now(),
		Action:    action,
		Source:    source,
		Count:     count,
		Details:   details,
	})
}

// GetHistory returns the history entries
func (s *Store) GetHistory() []HistoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := make([]HistoryEntry, len(s.state.History))
	copy(history, s.state.History)
	return history
}

// GetRecentHistory returns the last N history entries
func (s *Store) GetRecentHistory(n int) []HistoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if n >= len(s.state.History) {
		history := make([]HistoryEntry, len(s.state.History))
		copy(history, s.state.History)
		return history
	}

	start := len(s.state.History) - n
	history := make([]HistoryEntry, n)
	copy(history, s.state.History[start:])
	return history
}

// ImportProducts imports products, updating existing ones
func (s *Store) ImportProducts(products []models.EnhancedProduct, source string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, p := range products {
		product := p // Create copy
		if existing, exists := s.state.Products[p.SKU]; exists {
			// Merge with existing product
			product = *mergeProducts(existing, &product)
		}
		product.UpdatedAt = time.Now()
		s.state.Products[product.SKU] = &product
		count++
	}

	s.state.History = append(s.state.History, HistoryEntry{
		Timestamp: time.Now(),
		Action:    "import",
		Source:    source,
		Count:     count,
		Details:   fmt.Sprintf("Imported %d products from %s", count, source),
	})

	return count
}

// ImportLegacyProducts imports legacy Product types
func (s *Store) ImportLegacyProducts(products []models.Product, source string) int {
	enhanced := make([]models.EnhancedProduct, 0, len(products))
	for _, p := range products {
		enhanced = append(enhanced, *p.ToEnhancedProduct())
	}
	return s.ImportProducts(enhanced, source)
}

// ExportLegacyProducts exports products as legacy Product types
func (s *Store) ExportLegacyProducts() []models.Product {
	s.mu.RLock()
	defer s.mu.RUnlock()

	products := make([]models.Product, 0, len(s.state.Products))
	for _, p := range s.state.Products {
		products = append(products, *p.ToLegacyProduct())
	}
	return products
}

// mergeProducts merges a new product into an existing one
func mergeProducts(existing, new *models.EnhancedProduct) *models.EnhancedProduct {
	// Keep existing data but update with new data where available
	result := *existing

	// Update basic fields if new values are set
	if new.Title != "" && new.Title != existing.Title {
		result.Title = new.Title
	}
	if new.Description != "" && new.Description != existing.Description {
		result.Description = new.Description
	}
	if new.Handle != "" {
		result.Handle = new.Handle
	}
	if new.Barcode != "" {
		result.Barcode = new.Barcode
	}
	if new.NOBBNumber != "" {
		result.NOBBNumber = new.NOBBNumber
	}

	// Merge images (avoid duplicates)
	existingURLs := make(map[string]bool)
	for _, img := range existing.Images {
		existingURLs[img.SourceURL] = true
	}
	for _, img := range new.Images {
		if !existingURLs[img.SourceURL] {
			result.Images = append(result.Images, img)
		}
	}

	// Merge specifications
	if result.Specifications == nil {
		result.Specifications = make(map[string]string)
	}
	for k, v := range new.Specifications {
		result.Specifications[k] = v
	}

	// Merge properties (add new ones)
	existingProps := make(map[string]bool)
	for _, prop := range existing.Properties {
		existingProps[prop.Code] = true
	}
	for _, prop := range new.Properties {
		if !existingProps[prop.Code] {
			result.Properties = append(result.Properties, prop)
		}
	}

	// Keep enhancements from both
	result.Enhancements = append(result.Enhancements, new.Enhancements...)

	return &result
}

// DefaultStore is the global state store
var DefaultStore = NewStore("")

// Load loads the default store
func Load() error {
	return DefaultStore.Load()
}

// Save saves the default store
func Save() error {
	return DefaultStore.Save()
}
