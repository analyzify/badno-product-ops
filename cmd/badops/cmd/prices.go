package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/badno/badops/internal/database"
	"github.com/badno/badops/internal/database/postgres"
	"github.com/badno/badops/internal/prices"
	"github.com/badno/badops/pkg/models"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var pricesCmd = &cobra.Command{
	Use:   "prices",
	Short: "Price tracking commands",
	Long:  "Commands for importing and analyzing competitor prices",
}

var pricesImportCmd = &cobra.Command{
	Use:   "import <csv-file>",
	Short: "Import prices from Reprice CSV export",
	Long:  "Parses a Reprice CSV file and imports competitor prices into the database",
	Args:  cobra.ExactArgs(1),
	RunE:  runPricesImport,
}

var pricesCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check current prices for a product",
	Long:  "Shows the latest competitor prices for a specific product",
	RunE:  runPricesCheck,
}

var pricesSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show price data summary",
	Long:  "Shows summary statistics for price observations",
	RunE:  runPricesSummary,
}

var (
	pricesSKU     string
	pricesBarcode string
	pricesDays    int
)

func init() {
	pricesCmd.AddCommand(pricesImportCmd)
	pricesCmd.AddCommand(pricesCheckCmd)
	pricesCmd.AddCommand(pricesSummaryCmd)

	pricesCheckCmd.Flags().StringVar(&pricesSKU, "sku", "", "Product SKU to check")
	pricesCheckCmd.Flags().StringVar(&pricesBarcode, "barcode", "", "Product barcode to check")
	pricesCheckCmd.Flags().IntVar(&pricesDays, "days", 30, "Number of days of history to show")
}

func runPricesImport(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	csvFile := args[0]

	// Validate file exists
	if _, err := os.Stat(csvFile); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", csvFile)
	}

	fmt.Printf("Parsing: %s\n", filepath.Base(csvFile))

	// Parse CSV
	parser := prices.NewParser()
	result, err := parser.ParseFile(csvFile)
	if err != nil {
		return fmt.Errorf("failed to parse CSV: %w", err)
	}

	// Show parse summary
	fmt.Println("\n" + color.CyanString("Parse Summary"))
	fmt.Printf("  Products:     %d\n", result.ProductCount)
	fmt.Printf("  Competitors:  %d\n", len(result.Competitors))
	fmt.Printf("  Observations: %d\n", len(result.Records))

	if len(result.Errors) > 0 {
		color.Yellow("  Warnings:     %d", len(result.Errors))
		for _, e := range result.Errors[:min(5, len(result.Errors))] {
			fmt.Printf("    • %s\n", e)
		}
		if len(result.Errors) > 5 {
			fmt.Printf("    ... and %d more\n", len(result.Errors)-5)
		}
	}

	if len(result.Records) == 0 {
		color.Yellow("\nNo price observations found in file")
		return nil
	}

	// List detected competitors
	fmt.Println("\nDetected competitors:")
	for name := range result.Competitors {
		fmt.Printf("  • %s\n", name)
	}

	// Connect to database
	client, err := getDBClient()
	if err != nil {
		return err
	}

	fmt.Println("\nConnecting to database...")
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	color.Green("✓ Connected")

	// Get or create competitors
	fmt.Println("\nSyncing competitors...")
	competitorRepo := postgres.NewCompetitorRepo(client)
	competitorMap := make(map[string]int)

	for name := range result.Competitors {
		competitor, err := competitorRepo.GetOrCreate(ctx, name)
		if err != nil {
			return fmt.Errorf("failed to get/create competitor %s: %w", name, err)
		}
		competitorMap[name] = competitor.ID
	}
	color.Green("✓ %d competitors synced", len(competitorMap))

	// Build product map (SKU/barcode -> UUID)
	fmt.Println("\nBuilding product map...")
	productRepo := postgres.NewProductRepo(client)
	allProducts, err := productRepo.GetAll(ctx, database.QueryOptions{})
	if err != nil {
		return fmt.Errorf("failed to get products: %w", err)
	}

	productMap := make(map[string]uuid.UUID)
	for _, p := range allProducts {
		if p.ID != "" {
			id, err := uuid.Parse(p.ID)
			if err == nil {
				productMap[p.SKU] = id
				if p.Barcode != "" {
					productMap[p.Barcode] = id
				}
			}
		}
	}
	fmt.Printf("  Found %d products in database\n", len(allProducts))

	// Create competitor product links
	fmt.Println("\nCreating competitor product links...")
	links := prices.ConvertToCompetitorProducts(result.Records, productMap, competitorMap)
	if len(links) > 0 {
		linkRepo := postgres.NewCompetitorProductRepo(client)
		linkCount, err := linkRepo.BulkUpsert(ctx, links)
		if err != nil {
			return fmt.Errorf("failed to create links: %w", err)
		}
		color.Green("✓ %d product-competitor links created", linkCount)

		// Update competitor product counts
		if err := linkRepo.UpdateCompetitorProductCounts(ctx); err != nil {
			color.Yellow("Warning: failed to update competitor counts: %v", err)
		}
	}

	// Import price observations
	fmt.Println("\nImporting price observations...")
	observations := prices.ConvertToPriceObservations(result.Records, productMap, competitorMap)

	if len(observations) > 0 {
		priceRepo := postgres.NewPriceObservationRepo(client)

		// Use progress bar for large imports
		bar := progressbar.NewOptions(len(observations),
			progressbar.OptionSetDescription("Importing"),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowCount(),
			progressbar.OptionClearOnFinish(),
		)

		// Batch insert
		batchSize := 1000
		totalInserted := 0
		for i := 0; i < len(observations); i += batchSize {
			end := min(i+batchSize, len(observations))
			batch := observations[i:end]

			count, err := priceRepo.BulkCreate(ctx, batch)
			if err != nil {
				return fmt.Errorf("failed to insert observations: %w", err)
			}
			totalInserted += count
			bar.Add(len(batch))
		}

		bar.Finish()
		color.Green("✓ %d price observations imported", totalInserted)
	} else {
		color.Yellow("No matching products found for price import")
		fmt.Println("Ensure products are imported before importing prices")
	}

	// Log operation
	historyRepo := postgres.NewHistoryRepo(client)
	historyRepo.Add(ctx, &database.OperationHistory{
		Action:    "prices_import",
		Source:    filepath.Base(csvFile),
		Count:     len(observations),
		Details:   fmt.Sprintf("Imported %d observations from %d products across %d competitors", len(observations), result.ProductCount, len(result.Competitors)),
		StartedAt: time.Now(),
	})

	// Summary
	fmt.Println("\n" + color.CyanString("Import Summary"))
	fmt.Printf("  Products matched: %d/%d\n", countMatchedProducts(result.Records, productMap), result.ProductCount)
	fmt.Printf("  Competitors:      %d\n", len(competitorMap))
	fmt.Printf("  Links created:    %d\n", len(links))
	fmt.Printf("  Observations:     %d\n", len(observations))

	return nil
}

func runPricesCheck(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if pricesSKU == "" && pricesBarcode == "" {
		return fmt.Errorf("specify --sku or --barcode")
	}

	// Connect to database
	client, err := getDBClient()
	if err != nil {
		return err
	}

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	// Find product
	productRepo := postgres.NewProductRepo(client)
	var product *models.EnhancedProduct

	if pricesSKU != "" {
		product, err = productRepo.GetBySKU(ctx, pricesSKU)
	} else {
		product, err = productRepo.GetByBarcode(ctx, pricesBarcode)
	}

	if err != nil {
		return fmt.Errorf("failed to find product: %w", err)
	}
	if product == nil {
		return fmt.Errorf("product not found")
	}

	fmt.Printf("Product: %s\n", product.Title)
	fmt.Printf("SKU: %s\n", product.SKU)
	if product.Barcode != "" {
		fmt.Printf("Barcode: %s\n", product.Barcode)
	}
	if product.Price != nil {
		fmt.Printf("Our Price: %.2f %s\n", product.Price.Amount, product.Price.Currency)
	}

	// Get latest prices from competitors
	productID, _ := uuid.Parse(product.ID)
	priceRepo := postgres.NewPriceObservationRepo(client)
	latestPrices, err := priceRepo.GetLatestByProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to get prices: %w", err)
	}

	if len(latestPrices) == 0 {
		color.Yellow("\nNo competitor prices found")
		return nil
	}

	// Get competitor names
	competitorRepo := postgres.NewCompetitorRepo(client)
	competitors, _ := competitorRepo.GetAll(ctx)
	competitorNames := make(map[int]string)
	for _, c := range competitors {
		competitorNames[c.ID] = c.Name
	}

	// Display prices
	fmt.Println("\n" + color.CyanString("Competitor Prices"))

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Competitor", "Price", "Stock", "Last Updated"})
	table.SetBorder(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, p := range latestPrices {
		name := competitorNames[p.CompetitorID]
		if name == "" {
			name = fmt.Sprintf("Competitor %d", p.CompetitorID)
		}

		stock := "Yes"
		if !p.InStock {
			stock = color.RedString("No")
		}

		table.Append([]string{
			name,
			fmt.Sprintf("%.2f %s", p.Price, p.Currency),
			stock,
			p.ObservedAt.Format("2006-01-02 15:04"),
		})
	}

	table.Render()

	// Calculate market position
	if product.Price != nil && len(latestPrices) > 0 {
		var minPrice, maxPrice, sumPrice float64
		minPrice = latestPrices[0].Price
		for _, p := range latestPrices {
			if p.Price < minPrice {
				minPrice = p.Price
			}
			if p.Price > maxPrice {
				maxPrice = p.Price
			}
			sumPrice += p.Price
		}
		avgPrice := sumPrice / float64(len(latestPrices))

		fmt.Println("\n" + color.CyanString("Market Position"))
		fmt.Printf("  Market Min: %.2f\n", minPrice)
		fmt.Printf("  Market Max: %.2f\n", maxPrice)
		fmt.Printf("  Market Avg: %.2f\n", avgPrice)

		diff := ((product.Price.Amount - avgPrice) / avgPrice) * 100
		if diff > 0 {
			color.Yellow("  Our Price: %.1f%% above market average", diff)
		} else {
			color.Green("  Our Price: %.1f%% below market average", -diff)
		}
	}

	return nil
}

func runPricesSummary(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to database
	client, err := getDBClient()
	if err != nil {
		return err
	}

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	// Get counts
	priceRepo := postgres.NewPriceObservationRepo(client)
	obsCount, err := priceRepo.Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to count observations: %w", err)
	}

	competitorRepo := postgres.NewCompetitorRepo(client)
	competitors, err := competitorRepo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get competitors: %w", err)
	}

	linkRepo := postgres.NewCompetitorProductRepo(client)
	linkCount, err := linkRepo.Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to count links: %w", err)
	}

	// Summary
	fmt.Println(color.CyanString("Price Data Summary"))
	fmt.Printf("  Total Observations: %d\n", obsCount)
	fmt.Printf("  Competitors:        %d\n", len(competitors))
	fmt.Printf("  Product Links:      %d\n", linkCount)

	// Competitor breakdown
	if len(competitors) > 0 {
		fmt.Println("\n" + color.CyanString("Competitors"))

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Competitor", "Products", "Last Scraped"})
		table.SetBorder(false)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)

		for _, c := range competitors {
			lastScraped := "Never"
			if c.LastScraped != nil {
				lastScraped = c.LastScraped.Format("2006-01-02")
			}
			table.Append([]string{
				c.Name,
				fmt.Sprintf("%d", c.ProductCount),
				lastScraped,
			})
		}

		table.Render()
	}

	return nil
}

// Helper functions
func countMatchedProducts(records []prices.CSVRecord, productMap map[string]uuid.UUID) int {
	matched := make(map[string]bool)
	for _, rec := range records {
		if _, ok := productMap[rec.SKU]; ok {
			matched[rec.SKU] = true
		} else if rec.Barcode != "" {
			if _, ok := productMap[rec.Barcode]; ok {
				matched[rec.Barcode] = true
			}
		}
	}
	return len(matched)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
