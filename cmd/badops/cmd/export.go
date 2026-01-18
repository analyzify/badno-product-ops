package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/badno/badops/internal/config"
	"github.com/badno/badops/internal/output"
	"github.com/badno/badops/internal/output/file"
	"github.com/badno/badops/internal/state"
	"github.com/badno/badops/pkg/models"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	exportDest        string
	exportFormat      string
	exportOutputPath  string
	exportOnlyEnhanced bool
	exportDryRun      bool
	exportIncludeImages bool
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export products to various destinations",
	Long:  `Export enhanced products to CSV, JSON, Shopify, or ClickHouse.`,
}

var exportRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run export to destination",
	Long:  `Export products to the specified destination.`,
	RunE:  runExport,
}

var exportListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available export destinations",
	Long:  `Show all available export adapters.`,
	RunE:  runExportList,
}

func init() {
	exportRunCmd.Flags().StringVar(&exportDest, "dest", "csv", "Export destination (csv, json, shopify, clickhouse)")
	exportRunCmd.Flags().StringVar(&exportFormat, "format", "matrixify", "Output format (matrixify, shopify, json, jsonl)")
	exportRunCmd.Flags().StringVarP(&exportOutputPath, "output", "o", "", "Output file path (for file exports)")
	exportRunCmd.Flags().BoolVar(&exportOnlyEnhanced, "enhanced-only", false, "Only export enhanced products")
	exportRunCmd.Flags().BoolVar(&exportDryRun, "dry-run", false, "Preview without exporting")
	exportRunCmd.Flags().BoolVar(&exportIncludeImages, "images", true, "Include image URLs in export")

	exportCmd.AddCommand(exportRunCmd)
	exportCmd.AddCommand(exportListCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  EXPORTING PRODUCTS")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	// Load state
	store := state.NewStore("")
	if err := store.Load(); err != nil {
		color.Red("  Error loading state: %v", err)
		return err
	}

	// Get products
	products := store.GetAllProducts()
	if len(products) == 0 {
		color.Yellow("  No products found. Run 'badops products import' or 'badops products parse' first.")
		return nil
	}

	// Convert to slice of values
	productValues := make([]models.EnhancedProduct, 0, len(products))
	for _, p := range products {
		productValues = append(productValues, *p)
	}

	color.Yellow("  Found %d products\n", len(productValues))
	color.Yellow("  Destination: %s\n", exportDest)
	color.Yellow("  Format: %s\n", exportFormat)
	if exportDryRun {
		color.Yellow("  Mode: DRY RUN\n")
	}
	fmt.Println()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		color.Yellow("  Warning: Could not load config, using defaults")
		cfg = config.DefaultConfig()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get adapter
	var adapter output.Adapter
	switch exportDest {
	case "csv":
		adapter = file.NewCSVAdapter(file.CSVConfig{
			OutputDir: cfg.Outputs.File.OutputDir,
		})
	case "json":
		adapter = file.NewJSONAdapter(file.JSONConfig{
			OutputDir: cfg.Outputs.File.OutputDir,
			Pretty:    cfg.Outputs.File.Pretty,
		})
	default:
		color.Red("  Error: Unsupported destination: %s", exportDest)
		return fmt.Errorf("unsupported destination: %s", exportDest)
	}

	// Connect
	if err := adapter.Connect(ctx); err != nil {
		color.Red("  Error connecting to destination: %v", err)
		return err
	}
	defer adapter.Close()

	// Build export options
	opts := output.ExportOptions{
		Format:        output.Format(exportFormat),
		OutputPath:    exportOutputPath,
		IncludeImages: exportIncludeImages,
		OnlyEnhanced:  exportOnlyEnhanced,
		DryRun:        exportDryRun,
	}

	// Export
	result, err := adapter.ExportProducts(ctx, productValues, opts)
	if err != nil {
		color.Red("  Error during export: %v", err)
		return err
	}

	// Show result
	if result.Success {
		success.Printf("  ✓ Exported %d products\n", result.ProductsExported)
		if result.ImagesExported > 0 {
			success.Printf("  ✓ Exported %d images\n", result.ImagesExported)
		}
		if result.Destination != "" {
			success.Printf("  ✓ Output: %s\n", result.Destination)
		}
		success.Printf("  ✓ %s\n", result.Details)

		// Update history
		if !exportDryRun {
			store.AddHistory("export", exportDest, result.ProductsExported,
				fmt.Sprintf("Exported to %s", result.Destination))
			store.Save()
		}
	} else {
		color.Red("  Export failed: %v", result.Error)
	}
	fmt.Println()

	return nil
}

func runExportList(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)

	header.Println("\n  AVAILABLE EXPORT DESTINATIONS")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Destination", "Formats", "Description"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	destinations := []struct {
		name    string
		formats string
		desc    string
	}{
		{"csv", "matrixify, shopify", "CSV file export (Matrixify/Shopify format)"},
		{"json", "json, jsonl", "JSON file export"},
		{"shopify", "-", "Direct Shopify API update (requires API key)"},
		{"clickhouse", "-", "ClickHouse data warehouse"},
	}

	for _, d := range destinations {
		table.Append([]string{d.name, d.formats, d.desc})
	}

	table.Render()
	fmt.Println()

	color.Yellow("  Example usage:")
	fmt.Println("    badops export run --dest csv --format matrixify")
	fmt.Println("    badops export run --dest json --enhanced-only")
	fmt.Println("    badops export run --dest csv -o my-export.csv")
	fmt.Println()

	return nil
}
