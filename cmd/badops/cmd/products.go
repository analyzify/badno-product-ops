package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/badno/badops/internal/config"
	"github.com/badno/badops/internal/matcher"
	"github.com/badno/badops/internal/parser"
	"github.com/badno/badops/internal/source"
	"github.com/badno/badops/internal/source/shopify"
	"github.com/badno/badops/internal/state"
	"github.com/badno/badops/pkg/models"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	parsedProducts   []models.Product
	stateFile        = "output/.badops-state.json"
	importSource     string
	importLimit      int
	importVendor     string
)

var productsCmd = &cobra.Command{
	Use:   "products",
	Short: "Manage product data",
	Long:  `Parse, match, import, and manage product data.`,
}

var parseCmd = &cobra.Command{
	Use:   "parse [csv-file]",
	Short: "Parse a Matrixify CSV export",
	Long:  `Parse a Matrixify CSV export and extract Tiger brand products.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runParse,
}

var matchCmd = &cobra.Command{
	Use:   "match",
	Short: "Match products against Tiger.nl",
	Long:  `Match parsed products against the Tiger.nl product catalog.`,
	RunE:  runMatch,
}

var lookupCmd = &cobra.Command{
	Use:   "lookup [sku]",
	Short: "Look up a single SKU on Tiger.nl",
	Long:  `Look up a single SKU directly on Tiger.nl using ID-based matching.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runLookup,
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import products from a source",
	Long:  `Import products from Shopify or other configured sources.`,
	RunE:  runImport,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List products in state",
	Long:  `Display all products currently in the state file.`,
	RunE:  runList,
}

func init() {
	importCmd.Flags().StringVar(&importSource, "source", "shopify", "Source to import from (shopify)")
	importCmd.Flags().IntVar(&importLimit, "limit", 0, "Maximum products to import (0 = all)")
	importCmd.Flags().StringVar(&importVendor, "vendor", "", "Only import products from this vendor")

	productsCmd.AddCommand(parseCmd)
	productsCmd.AddCommand(matchCmd)
	productsCmd.AddCommand(lookupCmd)
	productsCmd.AddCommand(importCmd)
	productsCmd.AddCommand(listCmd)
}

func runParse(cmd *cobra.Command, args []string) error {
	csvFile := args[0]

	// Header
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)
	info := color.New(color.FgYellow)

	header.Println("\n  PARSING MATRIXIFY EXPORT")
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	// Check if file exists
	if _, err := os.Stat(csvFile); os.IsNotExist(err) {
		color.Red("  Error: File not found: %s", csvFile)
		return fmt.Errorf("file not found: %s", csvFile)
	}

	info.Printf("  Source: %s\n\n", csvFile)

	// Parse the CSV
	products, err := parser.ParseMatrixifyCSV(csvFile)
	if err != nil {
		color.Red("  Error parsing CSV: %v", err)
		return err
	}

	// Show progress bar for "processing"
	bar := progressbar.NewOptions(len(products),
		progressbar.OptionSetDescription("  Processing products"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        color.GreenString("█"),
			SaucerHead:    color.GreenString("█"),
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
	)

	for range products {
		bar.Add(1)
		time.Sleep(50 * time.Millisecond) // Simulate processing
	}
	fmt.Println()
	fmt.Println()

	// Display results table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "Product Name", "Images"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)
	table.SetColumnColor(
		tablewriter.Colors{tablewriter.FgYellowColor},
		tablewriter.Colors{},
		tablewriter.Colors{tablewriter.FgGreenColor},
	)

	for _, p := range products {
		imgCount := len(p.ExistingImages)
		imgStatus := fmt.Sprintf("%d", imgCount)
		if imgCount == 0 {
			imgStatus = color.RedString("missing")
		}
		// Truncate name if too long
		name := p.Name
		if len(name) > 35 {
			name = name[:32] + "..."
		}
		table.Append([]string{p.SKU, name, imgStatus})
	}
	table.Render()
	fmt.Println()

	// Summary
	withImages := 0
	missingImages := 0
	for _, p := range products {
		if len(p.ExistingImages) > 0 {
			withImages++
		} else {
			missingImages++
		}
	}

	success.Printf("  ✓ Parsed %d products\n", len(products))
	if missingImages > 0 {
		color.Yellow("  ⚠ %d products missing images\n", missingImages)
	}
	success.Printf("  ✓ %d products have existing images\n", withImages)
	fmt.Println()

	// Save state for next command
	if err := saveState(products); err != nil {
		color.Red("  Warning: Could not save state: %v", err)
	}

	return nil
}

func runMatch(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  MATCHING AGAINST TIGER.NL")
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	// Load products from state
	products, err := loadState()
	if err != nil || len(products) == 0 {
		color.Yellow("  No products loaded. Run 'badops products parse' first.")
		return fmt.Errorf("no products to match")
	}

	color.Yellow("  Found %d products to match\n\n", len(products))

	// Create matcher
	m := matcher.NewTigerMatcher()

	// Progress bar
	bar := progressbar.NewOptions(len(products),
		progressbar.OptionSetDescription("  Matching products"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        color.GreenString("█"),
			SaucerHead:    color.GreenString("█"),
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
	)

	matched := 0
	for i := range products {
		url, score := m.Match(products[i])
		products[i].MatchedURL = url
		products[i].MatchScore = score
		if score > 0.7 {
			matched++
		}
		bar.Add(1)
		time.Sleep(150 * time.Millisecond) // Simulate API call
	}
	fmt.Println()
	fmt.Println()

	// Display results table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "Product Name", "Match Score", "Status"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	for _, p := range products {
		name := p.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		scoreStr := fmt.Sprintf("%.0f%%", p.MatchScore*100)
		status := color.GreenString("matched")
		if p.MatchScore < 0.7 {
			status = color.YellowString("review")
		}
		if p.MatchScore < 0.5 {
			status = color.RedString("no match")
		}
		table.Append([]string{p.SKU, name, scoreStr, status})
	}
	table.Render()
	fmt.Println()

	// Summary
	success.Printf("  ✓ Matched %d/%d products (%.0f%%)\n", matched, len(products), float64(matched)/float64(len(products))*100)
	fmt.Println()

	// Save updated state
	if err := saveState(products); err != nil {
		color.Red("  Warning: Could not save state: %v", err)
	}

	// Save report
	if err := saveReport(products); err != nil {
		color.Red("  Warning: Could not save report: %v", err)
	} else {
		color.Green("  ✓ Report saved to output/report.json")
	}
	fmt.Println()

	return nil
}

func saveState(products []models.Product) error {
	// Use the new v2 state store
	store := state.NewStore("")
	store.Load() // Load existing state if any
	store.ImportLegacyProducts(products, "csv")
	return store.Save()
}

func loadState() ([]models.Product, error) {
	// Try to load from v2 state store first
	store := state.NewStore("")
	if err := store.Load(); err == nil {
		return store.ExportLegacyProducts(), nil
	}

	// Fall back to legacy v1 format
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, err
	}
	var products []models.Product
	if err := json.Unmarshal(data, &products); err != nil {
		return nil, err
	}
	return products, nil
}

func saveReport(products []models.Product) error {
	matched := 0
	imagesFound := 0
	for _, p := range products {
		if p.MatchScore > 0.7 {
			matched++
		}
		imagesFound += len(p.NewImages)
	}

	report := models.Report{
		TotalProducts:   len(products),
		MatchedProducts: matched,
		ImagesFound:     imagesFound,
		Products:        products,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("output/report.json", data, 0644)
}

func runLookup(cmd *cobra.Command, args []string) error {
	sku := args[0]

	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)
	info := color.New(color.FgYellow)

	header.Println("\n  TIGER.NL SKU LOOKUP")
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	info.Printf("  SKU: %s\n\n", sku)

	// Create matcher
	m := matcher.NewTigerMatcher()
	skuMapper := m.GetSKUMapper()

	// Get candidate IDs
	candidateIDs := skuMapper.MapSKU(sku)
	info.Printf("  Candidate Tiger.nl IDs:\n")
	for _, id := range candidateIDs {
		fmt.Printf("    - %s\n", id)
	}
	fmt.Println()

	// Try to find the product
	info.Println("  Searching Tiger.nl...")
	fmt.Println()

	product, err := m.LookupBySKU(sku, "")
	if err != nil {
		color.Red("  Error: %v", err)
		return err
	}

	if product == nil {
		color.Yellow("  Product not found on Tiger.nl")
		color.Yellow("  Try with a product name for better category matching:")
		color.Yellow("    badops products lookup %s --name \"Tiger Boston Hook\"", sku)
		return nil
	}

	// Display results
	success.Printf("  ✓ Found: %s\n", product.URL)
	success.Printf("  ✓ Images: %d (validated)\n", len(product.ImageURLs))
	fmt.Println()

	// Show first few image URLs
	if len(product.ImageURLs) > 0 {
		info.Println("  Image URLs:")
		maxShow := 5
		if len(product.ImageURLs) < maxShow {
			maxShow = len(product.ImageURLs)
		}
		for i := 0; i < maxShow; i++ {
			fmt.Printf("    %d. %s\n", i+1, product.ImageURLs[i])
		}
		if len(product.ImageURLs) > maxShow {
			fmt.Printf("    ... and %d more\n", len(product.ImageURLs)-maxShow)
		}
	}
	fmt.Println()

	return nil
}

func runImport(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  IMPORTING PRODUCTS")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	color.Yellow("  Source: %s\n", importSource)
	if importVendor != "" {
		color.Yellow("  Vendor filter: %s\n", importVendor)
	}
	if importLimit > 0 {
		color.Yellow("  Limit: %d\n", importLimit)
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

	switch importSource {
	case "shopify":
		return importFromShopify(ctx, cfg, header, success)
	default:
		color.Red("  Error: Unsupported source: %s", importSource)
		return fmt.Errorf("unsupported source: %s", importSource)
	}
}

func importFromShopify(ctx context.Context, cfg *config.Config, header, success *color.Color) error {
	// Create Shopify connector
	conn := shopify.NewConnector(shopify.Config{
		Store:     cfg.Sources.Shopify.Store,
		APIKeyEnv: cfg.Sources.Shopify.APIKeyEnv,
	})

	color.Yellow("  Connecting to %s.myshopify.com...\n", cfg.Sources.Shopify.Store)

	if err := conn.Connect(ctx); err != nil {
		color.Red("  Error connecting to Shopify: %v", err)
		color.Yellow("  Make sure %s environment variable is set", cfg.Sources.Shopify.APIKeyEnv)
		return err
	}
	defer conn.Close()

	success.Println("  ✓ Connected to Shopify")
	fmt.Println()

	// Fetch products
	color.Yellow("  Fetching products...")

	result, err := conn.FetchProducts(ctx, source.FetchOptions{
		Limit:  importLimit,
		Vendor: importVendor,
	})

	if err != nil {
		color.Red("  Error fetching products: %v", err)
		return err
	}

	products := result.Products
	fmt.Println()

	if len(products) == 0 {
		color.Yellow("  No products found matching criteria")
		return nil
	}

	// Progress bar for processing
	bar := progressbar.NewOptions(len(products),
		progressbar.OptionSetDescription("  Processing products"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        color.GreenString("█"),
			SaucerHead:    color.GreenString("█"),
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
	)

	for range products {
		bar.Add(1)
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Println()
	fmt.Println()

	// Display results table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "Title", "Vendor", "Images"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	// Show first 20 products
	displayCount := len(products)
	if displayCount > 20 {
		displayCount = 20
	}

	for i := 0; i < displayCount; i++ {
		p := products[i]
		title := p.Title
		if len(title) > 30 {
			title = title[:27] + "..."
		}
		imgCount := fmt.Sprintf("%d", len(p.Images))
		table.Append([]string{p.SKU, title, p.Vendor, imgCount})
	}

	if len(products) > 20 {
		table.Append([]string{"...", "...", "...", fmt.Sprintf("and %d more", len(products)-20)})
	}

	table.Render()
	fmt.Println()

	// Save to state
	store := state.NewStore("")
	if err := store.Load(); err != nil {
		color.Yellow("  Warning: Could not load existing state, creating new")
	}

	count := store.ImportProducts(products, "shopify")
	if err := store.Save(); err != nil {
		color.Red("  Error saving state: %v", err)
		return err
	}

	success.Printf("  ✓ Imported %d products from Shopify\n", count)
	success.Println("  ✓ State saved to output/.badops-state.json")
	color.Yellow("  → Run 'badops enhance run' to enhance products")
	fmt.Println()

	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)

	header.Println("\n  PRODUCTS IN STATE")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	// Load state
	store := state.NewStore("")
	if err := store.Load(); err != nil {
		color.Yellow("  No state file found. Run 'badops products parse' or 'badops products import' first.")
		return nil
	}

	products := store.GetAllProducts()
	if len(products) == 0 {
		color.Yellow("  No products in state.")
		return nil
	}

	color.Yellow("  Found %d products\n\n", len(products))

	// Display table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "Title", "Vendor", "Images", "Status"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	// Show first 30 products
	displayCount := len(products)
	if displayCount > 30 {
		displayCount = 30
	}

	for i := 0; i < displayCount; i++ {
		p := products[i]
		title := p.Title
		if len(title) > 25 {
			title = title[:22] + "..."
		}
		imgCount := fmt.Sprintf("%d", len(p.Images))
		status := string(p.Status)
		if p.Status == models.StatusEnhanced || p.Status == models.StatusApproved {
			status = color.GreenString(status)
		} else if p.Status == models.StatusFailed {
			status = color.RedString(status)
		}
		table.Append([]string{p.SKU, title, p.Vendor, imgCount, status})
	}

	if len(products) > 30 {
		table.Append([]string{"...", "...", "...", "...", fmt.Sprintf("and %d more", len(products)-30)})
	}

	table.Render()
	fmt.Println()

	// Show summary by status
	statusCounts := make(map[models.ProductStatus]int)
	for _, p := range products {
		statusCounts[p.Status]++
	}

	if len(statusCounts) > 1 {
		header.Println("  STATUS SUMMARY")
		for status, count := range statusCounts {
			fmt.Printf("    %s: %d\n", status, count)
		}
		fmt.Println()
	}

	// Show recent history
	history := store.GetRecentHistory(5)
	if len(history) > 0 {
		header.Println("  RECENT HISTORY")
		for _, h := range history {
			fmt.Printf("    %s - %s (%s): %s\n",
				h.Timestamp.Format("2006-01-02 15:04"),
				h.Action, h.Source, h.Details)
		}
		fmt.Println()
	}

	return nil
}
