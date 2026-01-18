package file

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/badno/badops/internal/output"
	"github.com/badno/badops/pkg/models"
)

const JSONAdapterName = "json"

// JSONConfig holds JSON file output configuration
type JSONConfig struct {
	OutputDir string // Directory for output files
	Pretty    bool   // Pretty-print JSON
}

// JSONAdapter implements the output.Adapter interface for JSON files
type JSONAdapter struct {
	*output.BaseAdapter
	config JSONConfig
}

// NewJSONAdapter creates a new JSON file adapter
func NewJSONAdapter(cfg JSONConfig) *JSONAdapter {
	if cfg.OutputDir == "" {
		cfg.OutputDir = "output"
	}

	return &JSONAdapter{
		BaseAdapter: output.NewBaseAdapter(
			JSONAdapterName,
			[]output.Format{output.FormatJSON, output.FormatJSONL},
		),
		config: cfg,
	}
}

// Connect creates the output directory
func (a *JSONAdapter) Connect(ctx context.Context) error {
	if err := os.MkdirAll(a.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	a.SetConnected(true)
	return nil
}

// Close cleans up resources
func (a *JSONAdapter) Close() error {
	a.SetConnected(false)
	return nil
}

// Test verifies the output directory is writable
func (a *JSONAdapter) Test(ctx context.Context) error {
	testFile := filepath.Join(a.config.OutputDir, ".test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("output directory not writable: %w", err)
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

// ExportProducts exports products to a JSON file
func (a *JSONAdapter) ExportProducts(ctx context.Context, products []models.EnhancedProduct, opts output.ExportOptions) (*output.ExportResult, error) {
	result := &output.ExportResult{
		StartedAt: time.Now(),
	}

	if !a.IsConnected() {
		if err := a.Connect(ctx); err != nil {
			result.Error = err
			return result, err
		}
	}

	// Filter products if needed
	filteredProducts := products
	if opts.OnlyEnhanced {
		filteredProducts = make([]models.EnhancedProduct, 0)
		for _, p := range products {
			if len(p.Enhancements) > 0 {
				filteredProducts = append(filteredProducts, p)
			}
		}
	}
	if len(opts.SKUs) > 0 {
		skuSet := make(map[string]bool)
		for _, sku := range opts.SKUs {
			skuSet[sku] = true
		}
		temp := make([]models.EnhancedProduct, 0)
		for _, p := range filteredProducts {
			if skuSet[p.SKU] {
				temp = append(temp, p)
			}
		}
		filteredProducts = temp
	}

	if opts.DryRun {
		result.ProductsExported = len(filteredProducts)
		result.Success = true
		result.Details = fmt.Sprintf("Dry run: would export %d products", len(filteredProducts))
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Determine filename and format
	filename := opts.OutputPath
	format := opts.Format
	if format == "" {
		format = output.FormatJSON
	}

	if filename == "" {
		timestamp := time.Now().Format("2006-01-02_150405")
		ext := ".json"
		if format == output.FormatJSONL {
			ext = ".jsonl"
		}
		filename = filepath.Join(a.config.OutputDir, fmt.Sprintf("products_%s%s", timestamp, ext))
	}

	// Count images
	imagesExported := 0
	for _, p := range filteredProducts {
		imagesExported += len(p.Images)
	}

	var err error
	switch format {
	case output.FormatJSONL:
		err = a.writeJSONL(filename, filteredProducts)
	default:
		err = a.writeJSON(filename, filteredProducts)
	}

	if err != nil {
		result.Error = err
		return result, err
	}

	result.Destination = filename
	result.ProductsExported = len(filteredProducts)
	result.ImagesExported = imagesExported
	result.Success = true
	result.Details = fmt.Sprintf("Exported %d products to %s", len(filteredProducts), filename)
	result.CompletedAt = time.Now()

	return result, nil
}

// writeJSON writes products as a JSON array
func (a *JSONAdapter) writeJSON(filename string, products []models.EnhancedProduct) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	if a.config.Pretty {
		encoder.SetIndent("", "  ")
	}

	// Wrap in an export envelope
	export := struct {
		Version    string                   `json:"version"`
		ExportedAt time.Time                `json:"exported_at"`
		Count      int                      `json:"count"`
		Products   []models.EnhancedProduct `json:"products"`
	}{
		Version:    "2.0",
		ExportedAt: time.Now(),
		Count:      len(products),
		Products:   products,
	}

	return encoder.Encode(export)
}

// writeJSONL writes products as JSON Lines (one object per line)
func (a *JSONAdapter) writeJSONL(filename string, products []models.EnhancedProduct) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	for _, p := range products {
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if _, err := writer.Write(data); err != nil {
			return err
		}
		if _, err := writer.WriteString("\n"); err != nil {
			return err
		}
	}

	return nil
}
