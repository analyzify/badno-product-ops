package images

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ImageURL represents an image URL with metadata
type ImageURL struct {
	URL string
	SKU string
}

// Fetcher handles downloading images
type Fetcher struct {
	client    *http.Client
	outputDir string
}

// NewFetcher creates a new image fetcher
func NewFetcher() *Fetcher {
	return &Fetcher{
		client:    &http.Client{},
		outputDir: "output/originals",
	}
}

// GetDemoImageURLs returns real Tiger.nl image URLs for the demo
func (f *Fetcher) GetDemoImageURLs() []ImageURL {
	// Real Tiger.nl PIM image URLs (1200px for high quality)
	// These map to actual bad.no Tiger products
	return []ImageURL{
		{
			// Tiger Boston Toalettrullholder RVS gepolijst
			URL: "https://tiger.nl/pim/528_2be183e7-9c32-419d-975e-f2b4aad4145a?width=1200&height=1200&format=jpg&quality=90",
			SKU: "CO-T309012",
		},
		{
			// Tiger Boston krok matt sort (zwart)
			URL: "https://tiger.nl/pim/528_2098839f-839b-433f-a2d0-b42a23fdfc55?width=1200&height=1200&format=jpg&quality=90",
			SKU: "CO-T309512",
		},
		{
			// Tiger Urban toalettbørste (zwart)
			URL: "https://tiger.nl/pim/528_33c1baa2-1951-482e-bfc3-021c1d85d77f?width=1200&height=1200&format=jpg&quality=90",
			SKU: "CO-T317312",
		},
		{
			// Tiger Boston dobbel håndklestang (RVS gepolijst)
			URL: "https://tiger.nl/pim/528_b72deb2e-c380-48ec-9cb6-8a1e9b8d0acb?width=1200&height=1200&format=jpg&quality=90",
			SKU: "CO-T308612",
		},
		{
			// Tiger 2-Store Dusjkurv (wit)
			URL: "https://tiger.nl/pim/528_3ecd131e-3371-40cb-a97e-cf0475e3a4cd?width=1200&height=1200&format=jpg&quality=90",
			SKU: "CO-800380",
		},
	}
}

// Download fetches an image and saves it locally
func (f *Fetcher) Download(url, sku string) (string, string, error) {
	// Create output directory
	if err := os.MkdirAll(f.outputDir, 0755); err != nil {
		return "", "", err
	}

	// Determine file extension
	ext := ".jpg"
	if strings.Contains(url, ".png") {
		ext = ".png"
	} else if strings.Contains(url, ".webp") {
		ext = ".webp"
	}

	filename := fmt.Sprintf("%s%s", sku, ext)
	destPath := filepath.Join(f.outputDir, filename)

	// Download image
	resp, err := f.client.Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to download: HTTP %d", resp.StatusCode)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return "", "", err
	}
	defer out.Close()

	// Copy content
	n, err := io.Copy(out, resp.Body)
	if err != nil {
		return "", "", err
	}

	// Format size
	size := formatSize(n)

	return destPath, size, nil
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ValidateURL checks if a URL is accessible (returns HTTP 200)
// This is useful for pre-validating image URLs before downloading
func (f *Fetcher) ValidateURL(url string) (bool, error) {
	resp, err := f.client.Head(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// DownloadWithValidation downloads an image only if it passes validation
func (f *Fetcher) DownloadWithValidation(url, sku string) (string, string, error) {
	// First validate the URL
	valid, err := f.ValidateURL(url)
	if err != nil {
		return "", "", fmt.Errorf("validation failed: %w", err)
	}
	if !valid {
		return "", "", fmt.Errorf("URL returned non-200 status")
	}

	// Proceed with download
	return f.Download(url, sku)
}
