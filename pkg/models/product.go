package models

// Product represents a product from the Matrixify export
type Product struct {
	SKU            string   `json:"sku"`
	Name           string   `json:"name"`
	Brand          string   `json:"brand"`
	ExistingImages []string `json:"existing_images"`
	MatchedURL     string   `json:"matched_url,omitempty"`
	MatchScore     float64  `json:"match_score,omitempty"`
	NewImages      []Image  `json:"new_images,omitempty"`
}

// Image represents an image to be processed
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
