package matcher

import (
	"strings"
)

// SKUMapper maps bad.no SKUs to Tiger.nl product IDs
type SKUMapper struct {
	// colorSuffixMap maps bad.no color suffixes to Tiger.nl color suffixes
	// For T-prefixed SKUs: last 2 digits → 3-digit Tiger.nl suffix
	colorSuffixMap map[string]string
}

// NewSKUMapper creates a new SKU mapper with color mappings
func NewSKUMapper() *SKUMapper {
	return &SKUMapper{
		colorSuffixMap: map[string]string{
			// Bad.no suffix → Tiger.nl suffix
			"12": "746", // Black (svart)
			"32": "946", // Brushed stainless (mattbørstet)
			"33": "346", // Chrome (krom)
			"01": "146", // White (hvit)
			"41": "341", // Chrome variant
			"46": "146", // White variant
		},
	}
}

// MapSKU returns a list of candidate Tiger.nl IDs to try for a given bad.no SKU
// It returns multiple candidates because the exact mapping may vary
func (m *SKUMapper) MapSKU(badnoSKU string) []string {
	// Remove CO- prefix
	sku := strings.TrimPrefix(badnoSKU, "CO-")

	// Type 1: Numeric SKUs (e.g., 1500010746) - direct match
	if !strings.HasPrefix(sku, "T") {
		return []string{sku}
	}

	// Remove T prefix
	sku = strings.TrimPrefix(sku, "T")

	// Type 3: Cooper/800xxx series - direct numeric
	if strings.HasPrefix(sku, "800") {
		return []string{sku}
	}

	// Type 2: T-prefixed SKUs (e.g., T309012 → 309030346)
	// Pattern discovered:
	// - CO-T309012 → 309030346 (Boston toilet roll holder, polished)
	// - CO-T309512 → 309530746 (Boston hook, black)
	// - CO-T308612 → 308630746 (Boston towel rail, black)
	// - CO-T317312 → 1317330746 (Urban toilet brush, black)
	//
	// Rule: base(4 digits) + "3" + "0" + color_suffix(3 digits)
	// The 5th digit in bad.no (always 1?) transforms to 3 in Tiger.nl
	if len(sku) < 6 {
		return []string{sku}
	}

	base := sku[:4] // e.g., "3090" from "309012"
	// Note: the 5th digit (index 4) in bad.no is usually 1, but maps to 3 in Tiger.nl

	// Build candidates with different color suffixes
	// Pattern: base + "30" + color_suffix (e.g., "3090" + "30" + "746" = "309030746")
	var candidates []string

	// Try all color variants since the color mapping from bad.no isn't reliable
	for _, suffix := range []string{"746", "346", "946", "146"} {
		id := base + "30" + suffix
		candidates = append(candidates, id)
	}

	// Also try with "50" pattern (seen in some products like T309512 → 309530746)
	for _, suffix := range []string{"746", "346", "946", "146"} {
		id := base[:3] + sku[3:5] + "30" + suffix // e.g., "309" + "51" → "30953" + "0" + suffix
		if !contains(candidates, id) {
			candidates = append(candidates, id)
		}
	}

	// Try prefixed variants for Urban series (T317312 → 1317330746)
	if len(sku) >= 6 {
		for _, suffix := range []string{"746", "346", "946", "146"} {
			// Add "1" prefix: 317312 → 1317330746
			id := "1" + base[:3] + sku[3:5] + "30" + suffix
			if !contains(candidates, id) {
				candidates = append(candidates, id)
			}
		}
	}

	// Fallback: try the raw numeric part
	if !contains(candidates, sku) {
		candidates = append(candidates, sku)
	}

	return candidates
}

// GetCategoryPath returns the base URL path for a product based on series name
func (m *SKUMapper) GetCategoryPath(productName string) string {
	nameLower := strings.ToLower(productName)

	// Universal hooks category (different URL structure)
	universalHooksSeries := []string{"baseline", "basic", "twin", "rhino", "cats"}
	for _, series := range universalHooksSeries {
		if strings.Contains(nameLower, series) {
			return "/en/products/universal-hooks/"
		}
	}

	// Default: bathroom accessories
	return "/producten/badkameraccessoires/"
}

// GetProductTypes returns the list of product type slugs to try
func (m *SKUMapper) GetProductTypes() []string {
	return []string{
		"toiletrolhouder",
		"toiletborstel-met-houder",
		"handdoekhouder",
		"haak",
		"douchemand",
		"zeepdispenser",
		"reserve-toiletrolhouder",
		"planchet",
		"bekerhouder",
		"badgreep",
		"spiegel",
		"accessoireset",
		"douchehaak",
		"schroefhaak",
		"opbergmand",
		// Universal hooks types
		"adhesive-hook",
		"screw-hook",
		"suction-hook",
		"door-hook",
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
