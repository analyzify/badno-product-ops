package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/badno/badops/internal/config"
	"github.com/badno/badops/internal/output"
	"github.com/badno/badops/internal/output/file"
	"github.com/badno/badops/internal/source"
	"github.com/badno/badops/internal/source/nobb"
	"github.com/badno/badops/internal/source/shopify"
	"github.com/badno/badops/internal/source/tiger"
	"github.com/badno/badops/internal/state"
	"github.com/badno/badops/pkg/models"
)

// Orchestrator coordinates the product enhancement pipeline
type Orchestrator struct {
	store     *state.Store
	config    *config.Config
	sources   map[string]source.Connector
	outputs   map[string]output.Adapter
}

// New creates a new orchestrator
func New(cfg *config.Config) *Orchestrator {
	return &Orchestrator{
		store:   state.NewStore(""),
		config:  cfg,
		sources: make(map[string]source.Connector),
		outputs: make(map[string]output.Adapter),
	}
}

// Initialize sets up all connectors and adapters
func (o *Orchestrator) Initialize(ctx context.Context) error {
	// Load state
	if err := o.store.Load(); err != nil {
		// Not fatal, just means starting fresh
	}

	// Initialize source connectors
	o.sources["shopify"] = shopify.NewConnector(shopify.Config{
		Store:     o.config.Sources.Shopify.Store,
		APIKeyEnv: o.config.Sources.Shopify.APIKeyEnv,
	})

	o.sources["nobb"] = nobb.NewConnector(nobb.Config{
		UsernameEnv: o.config.Sources.NOBB.UsernameEnv,
		PasswordEnv: o.config.Sources.NOBB.PasswordEnv,
	})

	o.sources["tiger_nl"] = tiger.NewConnector(tiger.Config{
		RateLimitMs: o.config.Sources.TigerNL.RateLimitMs,
	})

	// Initialize output adapters
	o.outputs["csv"] = file.NewCSVAdapter(file.CSVConfig{
		OutputDir: o.config.Outputs.File.OutputDir,
	})

	o.outputs["json"] = file.NewJSONAdapter(file.JSONConfig{
		OutputDir: o.config.Outputs.File.OutputDir,
		Pretty:    o.config.Outputs.File.Pretty,
	})

	return nil
}

// Close cleans up all resources
func (o *Orchestrator) Close() error {
	for _, s := range o.sources {
		s.Close()
	}
	for _, a := range o.outputs {
		a.Close()
	}
	return nil
}

// ImportOptions configures the import operation
type ImportOptions struct {
	Source string
	Vendor string
	Limit  int
}

// Import imports products from a source
func (o *Orchestrator) Import(ctx context.Context, opts ImportOptions) (*ImportResult, error) {
	result := &ImportResult{
		StartedAt: time.Now(),
	}

	// Get source connector
	src, ok := o.sources[opts.Source]
	if !ok {
		return nil, fmt.Errorf("unknown source: %s", opts.Source)
	}

	// Connect
	if err := src.Connect(ctx); err != nil {
		result.Error = err
		return result, err
	}

	// Fetch products
	fetchResult, err := src.FetchProducts(ctx, source.FetchOptions{
		Limit:  opts.Limit,
		Vendor: opts.Vendor,
	})
	if err != nil {
		result.Error = err
		return result, err
	}

	// Import to state
	count := o.store.ImportProducts(fetchResult.Products, opts.Source)

	// Save state
	if err := o.store.Save(); err != nil {
		result.Error = err
		return result, err
	}

	result.ProductsImported = count
	result.Success = true
	result.CompletedAt = time.Now()

	return result, nil
}

// ImportResult contains the results of an import operation
type ImportResult struct {
	ProductsImported int
	Success          bool
	Error            error
	StartedAt        time.Time
	CompletedAt      time.Time
}

// EnhanceOptions configures the enhancement operation
type EnhanceOptions struct {
	Sources []string
	Vendor  string
	Limit   int
	DryRun  bool
}

// Enhance runs enhancements on products
func (o *Orchestrator) Enhance(ctx context.Context, opts EnhanceOptions) (*EnhanceResult, error) {
	result := &EnhanceResult{
		StartedAt:  time.Now(),
		BySource:   make(map[string]int),
	}

	// Get products
	products := o.store.GetAllProducts()
	if opts.Vendor != "" {
		products = o.store.GetProductsByVendor(opts.Vendor)
	}
	if opts.Limit > 0 && opts.Limit < len(products) {
		products = products[:opts.Limit]
	}

	result.ProductsProcessed = len(products)

	// Connect to enhancement sources
	enhancers := make([]source.Connector, 0)
	for _, srcName := range opts.Sources {
		src, ok := o.sources[srcName]
		if !ok {
			continue
		}
		if err := src.Connect(ctx); err != nil {
			continue
		}
		enhancers = append(enhancers, src)
	}

	if len(enhancers) == 0 {
		return nil, fmt.Errorf("no enhancement sources available")
	}

	// Enhance each product
	for _, p := range products {
		for _, enhancer := range enhancers {
			if opts.DryRun {
				result.BySource[enhancer.Name()]++
				continue
			}

			enhResult, err := enhancer.EnhanceProduct(ctx, p)
			if err != nil {
				continue
			}
			if enhResult.Success {
				result.ProductsEnhanced++
				result.ImagesAdded += enhResult.ImagesAdded
				result.FieldsUpdated += len(enhResult.FieldsUpdated)
				result.BySource[enhancer.Name()]++
			}
		}
	}

	// Save state
	if !opts.DryRun {
		if err := o.store.Save(); err != nil {
			result.Error = err
			return result, err
		}
	}

	result.Success = true
	result.CompletedAt = time.Now()

	return result, nil
}

// EnhanceResult contains the results of an enhancement operation
type EnhanceResult struct {
	ProductsProcessed int
	ProductsEnhanced  int
	ImagesAdded       int
	FieldsUpdated     int
	BySource          map[string]int
	Success           bool
	Error             error
	StartedAt         time.Time
	CompletedAt       time.Time
}

// ExportOptions configures the export operation
type ExportOptions struct {
	Destination   string
	Format        output.Format
	OutputPath    string
	OnlyEnhanced  bool
	IncludeImages bool
	DryRun        bool
}

// Export exports products to a destination
func (o *Orchestrator) Export(ctx context.Context, opts ExportOptions) (*output.ExportResult, error) {
	// Get output adapter
	adapter, ok := o.outputs[opts.Destination]
	if !ok {
		return nil, fmt.Errorf("unknown destination: %s", opts.Destination)
	}

	// Connect
	if err := adapter.Connect(ctx); err != nil {
		return nil, err
	}

	// Get products
	products := o.store.GetAllProducts()
	productValues := make([]models.EnhancedProduct, 0, len(products))
	for _, p := range products {
		productValues = append(productValues, *p)
	}

	// Export
	return adapter.ExportProducts(ctx, productValues, output.ExportOptions{
		Format:        opts.Format,
		OutputPath:    opts.OutputPath,
		OnlyEnhanced:  opts.OnlyEnhanced,
		IncludeImages: opts.IncludeImages,
		DryRun:        opts.DryRun,
	})
}

// PipelineOptions configures a full pipeline run
type PipelineOptions struct {
	ImportSource    string
	ImportVendor    string
	ImportLimit     int
	EnhanceSources  []string
	ExportDest      string
	ExportFormat    output.Format
	ExportPath      string
	IncludeImages   bool
	DryRun          bool
}

// PipelineResult contains the results of a full pipeline run
type PipelineResult struct {
	Import      *ImportResult
	Enhance     *EnhanceResult
	Export      *output.ExportResult
	Success     bool
	Error       error
	StartedAt   time.Time
	CompletedAt time.Time
}

// RunPipeline executes a full import → enhance → export pipeline
func (o *Orchestrator) RunPipeline(ctx context.Context, opts PipelineOptions) (*PipelineResult, error) {
	result := &PipelineResult{
		StartedAt: time.Now(),
	}

	// Step 1: Import
	if opts.ImportSource != "" {
		importResult, err := o.Import(ctx, ImportOptions{
			Source: opts.ImportSource,
			Vendor: opts.ImportVendor,
			Limit:  opts.ImportLimit,
		})
		result.Import = importResult
		if err != nil {
			result.Error = fmt.Errorf("import failed: %w", err)
			return result, result.Error
		}
	}

	// Step 2: Enhance
	if len(opts.EnhanceSources) > 0 {
		enhanceResult, err := o.Enhance(ctx, EnhanceOptions{
			Sources: opts.EnhanceSources,
			DryRun:  opts.DryRun,
		})
		result.Enhance = enhanceResult
		if err != nil {
			result.Error = fmt.Errorf("enhance failed: %w", err)
			return result, result.Error
		}
	}

	// Step 3: Export
	if opts.ExportDest != "" {
		exportResult, err := o.Export(ctx, ExportOptions{
			Destination:   opts.ExportDest,
			Format:        opts.ExportFormat,
			OutputPath:    opts.ExportPath,
			IncludeImages: opts.IncludeImages,
			DryRun:        opts.DryRun,
		})
		result.Export = exportResult
		if err != nil {
			result.Error = fmt.Errorf("export failed: %w", err)
			return result, result.Error
		}
	}

	result.Success = true
	result.CompletedAt = time.Now()

	return result, nil
}

// GetStore returns the state store
func (o *Orchestrator) GetStore() *state.Store {
	return o.store
}

// GetSource returns a source connector by name
func (o *Orchestrator) GetSource(name string) (source.Connector, bool) {
	src, ok := o.sources[name]
	return src, ok
}

// GetOutput returns an output adapter by name
func (o *Orchestrator) GetOutput(name string) (output.Adapter, bool) {
	out, ok := o.outputs[name]
	return out, ok
}
