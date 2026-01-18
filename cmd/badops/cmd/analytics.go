package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/badno/badops/internal/config"
	"github.com/badno/badops/internal/database"
	"github.com/badno/badops/internal/database/clickhouse"
	"github.com/badno/badops/internal/database/postgres"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Price analytics commands",
	Long:  "Commands for analyzing price data and market positioning",
}

var analyticsTrendsCmd = &cobra.Command{
	Use:   "trends",
	Short: "Show price trends",
	Long:  "Displays price trends over time for products",
	RunE:  runAnalyticsTrends,
}

var analyticsPositionCmd = &cobra.Command{
	Use:   "position",
	Short: "Show market position",
	Long:  "Analyzes a product's market position relative to competitors",
	RunE:  runAnalyticsPosition,
}

var analyticsAlertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Show price alerts",
	Long:  "Lists products where price significantly differs from market average",
	RunE:  runAnalyticsAlerts,
}

var analyticsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync data to ClickHouse",
	Long:  "Synchronizes price data from PostgreSQL to ClickHouse for analytics",
	RunE:  runAnalyticsSync,
}

var analyticsInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize analytics database",
	Long:  "Creates ClickHouse tables and materialized views for analytics",
	RunE:  runAnalyticsInit,
}

var (
	analyticsPeriod    string
	analyticsVendor    string
	analyticsSKU       string
	analyticsThreshold float64
	analyticsSyncDays  int
	analyticsSyncAll   bool
)

func init() {
	analyticsCmd.AddCommand(analyticsTrendsCmd)
	analyticsCmd.AddCommand(analyticsPositionCmd)
	analyticsCmd.AddCommand(analyticsAlertsCmd)
	analyticsCmd.AddCommand(analyticsSyncCmd)
	analyticsCmd.AddCommand(analyticsInitCmd)

	analyticsTrendsCmd.Flags().StringVar(&analyticsPeriod, "period", "30d", "Time period (e.g., 7d, 30d, 90d)")
	analyticsTrendsCmd.Flags().StringVar(&analyticsVendor, "vendor", "", "Filter by vendor")
	analyticsTrendsCmd.Flags().StringVar(&analyticsSKU, "sku", "", "Filter by SKU")

	analyticsPositionCmd.Flags().StringVar(&analyticsSKU, "sku", "", "Product SKU to analyze (required)")
	analyticsPositionCmd.MarkFlagRequired("sku")

	analyticsAlertsCmd.Flags().Float64Var(&analyticsThreshold, "threshold", 10.0, "Price difference threshold in percent")
	analyticsAlertsCmd.Flags().StringVar(&analyticsVendor, "vendor", "", "Filter by vendor")

	analyticsSyncCmd.Flags().IntVar(&analyticsSyncDays, "days", 0, "Sync last N days (0 = incremental)")
	analyticsSyncCmd.Flags().BoolVar(&analyticsSyncAll, "all", false, "Sync all historical data")
}

// getClickHouseClient creates a ClickHouse client from configuration
func getClickHouseClient() (*clickhouse.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	chConfig := &clickhouse.Config{
		Host:     cfg.Database.ClickHouse.Host,
		Port:     cfg.Database.ClickHouse.Port,
		Database: cfg.Database.ClickHouse.Database,
		Username: os.Getenv(cfg.Database.ClickHouse.UsernameEnv),
		Password: os.Getenv(cfg.Database.ClickHouse.PasswordEnv),
		Secure:   cfg.Database.ClickHouse.Secure,
	}

	return clickhouse.NewClient(chConfig), nil
}

func parsePeriod(period string) int {
	// Parse period like "7d", "30d", "90d"
	var days int
	fmt.Sscanf(period, "%dd", &days)
	if days <= 0 {
		days = 30 // default
	}
	return days
}

func runAnalyticsTrends(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	days := parsePeriod(analyticsPeriod)

	// Connect to ClickHouse
	client, err := getClickHouseClient()
	if err != nil {
		return err
	}

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}
	defer client.Close()

	color.Green("✓ Connected to ClickHouse")

	var trends []clickhouse.PriceTrend

	if analyticsSKU != "" {
		// Get trends for specific SKU
		trends, err = client.GetPriceTrends(ctx, analyticsSKU, days)
		if err != nil {
			return fmt.Errorf("failed to get trends: %w", err)
		}
		fmt.Printf("\nPrice trends for %s (last %d days):\n\n", analyticsSKU, days)
	} else {
		// Get trends for vendor or all
		trends, err = client.GetVendorTrends(ctx, analyticsVendor, days)
		if err != nil {
			return fmt.Errorf("failed to get vendor trends: %w", err)
		}
		fmt.Printf("\nPrice trends (last %d days):\n\n", days)
	}

	if len(trends) == 0 {
		color.Yellow("No trend data found")
		fmt.Println("\nEnsure data is synced to ClickHouse:")
		fmt.Println("  badops analytics sync --all")
		return nil
	}

	// Group by product
	productTrends := make(map[string][]clickhouse.PriceTrend)
	for _, t := range trends {
		productTrends[t.ProductSKU] = append(productTrends[t.ProductSKU], t)
	}

	// Display summary for each product
	for sku, pts := range productTrends {
		if len(pts) == 0 {
			continue
		}

		fmt.Printf("%s\n", color.CyanString(sku))

		// Group by competitor
		competitorData := make(map[string]struct {
			firstPrice float64
			lastPrice  float64
			minPrice   float64
			maxPrice   float64
		})

		for _, t := range pts {
			data := competitorData[t.CompetitorName]
			if data.firstPrice == 0 {
				data.firstPrice = t.AvgPrice
			}
			data.lastPrice = t.AvgPrice
			if data.minPrice == 0 || t.MinPrice < data.minPrice {
				data.minPrice = t.MinPrice
			}
			if t.MaxPrice > data.maxPrice {
				data.maxPrice = t.MaxPrice
			}
			competitorData[t.CompetitorName] = data
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Competitor", "Current", "Min", "Max", "Change"})
		table.SetBorder(false)

		for competitor, data := range competitorData {
			change := ((data.lastPrice - data.firstPrice) / data.firstPrice) * 100
			changeStr := fmt.Sprintf("%.1f%%", change)
			if change > 0 {
				changeStr = color.RedString("+%.1f%%", change)
			} else if change < 0 {
				changeStr = color.GreenString("%.1f%%", change)
			}

			table.Append([]string{
				competitor,
				fmt.Sprintf("%.2f", data.lastPrice),
				fmt.Sprintf("%.2f", data.minPrice),
				fmt.Sprintf("%.2f", data.maxPrice),
				changeStr,
			})
		}

		table.Render()
		fmt.Println()
	}

	return nil
}

func runAnalyticsPosition(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Connect to both databases
	pgClient, err := getDBClient()
	if err != nil {
		return err
	}
	if err := pgClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer pgClient.Close()

	chClient, err := getClickHouseClient()
	if err != nil {
		return err
	}
	if err := chClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}
	defer chClient.Close()

	// Get product from PostgreSQL
	productRepo := postgres.NewProductRepo(pgClient)
	product, err := productRepo.GetBySKU(ctx, analyticsSKU)
	if err != nil {
		return fmt.Errorf("failed to get product: %w", err)
	}
	if product == nil {
		return fmt.Errorf("product not found: %s", analyticsSKU)
	}

	fmt.Printf("Product: %s\n", product.Title)
	fmt.Printf("SKU: %s\n", product.SKU)
	if product.Price != nil {
		fmt.Printf("Our Price: %.2f %s\n", product.Price.Amount, product.Price.Currency)
	}

	// Get price distribution from ClickHouse
	distribution, err := chClient.GetPriceDistribution(ctx, analyticsSKU)
	if err != nil {
		return fmt.Errorf("failed to get distribution: %w", err)
	}

	if len(distribution) == 0 {
		color.Yellow("\nNo competitor price data found")
		return nil
	}

	// Sort competitors by price
	type competitorPrice struct {
		name  string
		price float64
	}
	var sortedPrices []competitorPrice
	for name, price := range distribution {
		sortedPrices = append(sortedPrices, competitorPrice{name, price})
	}
	sort.Slice(sortedPrices, func(i, j int) bool {
		return sortedPrices[i].price < sortedPrices[j].price
	})

	fmt.Println("\n" + color.CyanString("Competitor Prices (Sorted)"))

	var minPrice, maxPrice, sumPrice float64
	minPrice = sortedPrices[0].price
	for i, cp := range sortedPrices {
		if cp.price < minPrice {
			minPrice = cp.price
		}
		if cp.price > maxPrice {
			maxPrice = cp.price
		}
		sumPrice += cp.price

		fmt.Printf("  %d. %s: %.2f\n", i+1, cp.name, cp.price)
	}

	avgPrice := sumPrice / float64(len(sortedPrices))

	fmt.Println("\n" + color.CyanString("Market Summary"))
	fmt.Printf("  Competitors: %d\n", len(distribution))
	fmt.Printf("  Market Min:  %.2f\n", minPrice)
	fmt.Printf("  Market Max:  %.2f\n", maxPrice)
	fmt.Printf("  Market Avg:  %.2f\n", avgPrice)

	if product.Price != nil {
		ownPrice := product.Price.Amount
		diff := ((ownPrice - avgPrice) / avgPrice) * 100

		fmt.Println("\n" + color.CyanString("Our Position"))
		if diff > 5 {
			color.Yellow("  %.1f%% ABOVE market average", diff)
		} else if diff < -5 {
			color.Green("  %.1f%% BELOW market average", -diff)
		} else {
			fmt.Printf("  At market average (%.1f%%)\n", diff)
		}

		// Find rank
		rank := 1
		for _, cp := range sortedPrices {
			if ownPrice > cp.price {
				rank++
			}
		}
		fmt.Printf("  Rank: %d of %d (1 = cheapest)\n", rank, len(sortedPrices)+1)
	}

	return nil
}

func runAnalyticsAlerts(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Connect to PostgreSQL to get our prices
	pgClient, err := getDBClient()
	if err != nil {
		return err
	}
	if err := pgClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer pgClient.Close()

	// Get all products with prices
	productRepo := postgres.NewProductRepo(pgClient)
	opts := database.QueryOptions{}
	if analyticsVendor != "" {
		opts.Vendor = analyticsVendor
	}

	products, err := productRepo.GetAll(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to get products: %w", err)
	}

	ownPrices := make(map[string]float64)
	for _, p := range products {
		if p.Price != nil && p.Price.Amount > 0 {
			ownPrices[p.SKU] = p.Price.Amount
		}
	}

	if len(ownPrices) == 0 {
		color.Yellow("No products with prices found")
		return nil
	}

	// Connect to ClickHouse
	chClient, err := getClickHouseClient()
	if err != nil {
		return err
	}
	if err := chClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}
	defer chClient.Close()

	// Get alerts
	alerts, err := chClient.GetPriceAlerts(ctx, analyticsThreshold, ownPrices)
	if err != nil {
		return fmt.Errorf("failed to get alerts: %w", err)
	}

	if len(alerts) == 0 {
		color.Green("✓ No price alerts (all products within %.0f%% of market average)", analyticsThreshold)
		return nil
	}

	// Sort by difference percentage
	sort.Slice(alerts, func(i, j int) bool {
		return alerts[i].DiffPercent > alerts[j].DiffPercent
	})

	fmt.Printf("Found %d products with price difference > %.0f%%:\n\n", len(alerts), analyticsThreshold)

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "Our Price", "Market Avg", "Difference"})
	table.SetBorder(false)

	aboveMarket := 0
	belowMarket := 0

	for _, a := range alerts {
		diffStr := fmt.Sprintf("%.1f%%", a.DiffPercent)
		if a.DiffPercent > 0 {
			diffStr = color.RedString("+%.1f%%", a.DiffPercent)
			aboveMarket++
		} else {
			diffStr = color.GreenString("%.1f%%", a.DiffPercent)
			belowMarket++
		}

		table.Append([]string{
			a.ProductSKU,
			fmt.Sprintf("%.2f", a.CurrentPrice),
			fmt.Sprintf("%.2f", a.MarketAvg),
			diffStr,
		})
	}

	table.Render()

	fmt.Printf("\nSummary: %d above market, %d below market\n", aboveMarket, belowMarket)

	return nil
}

func runAnalyticsSync(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Connect to PostgreSQL
	pgClient, err := getDBClient()
	if err != nil {
		return err
	}
	if err := pgClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer pgClient.Close()

	color.Green("✓ Connected to PostgreSQL")

	// Connect to ClickHouse
	chClient, err := getClickHouseClient()
	if err != nil {
		return err
	}
	if err := chClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}
	defer chClient.Close()

	color.Green("✓ Connected to ClickHouse")

	// Create syncer
	syncer := clickhouse.NewSyncer(pgClient, chClient)

	// Get stats before sync
	statsBefore, _ := syncer.GetSyncStats(ctx)
	fmt.Printf("\nPostgreSQL records: %d\n", statsBefore.TotalPGRecords)
	fmt.Printf("ClickHouse records: %d\n", statsBefore.TotalCHRecords)

	// Determine sync mode
	var result *clickhouse.SyncResult
	fmt.Println("\nSyncing...")

	bar := progressbar.NewOptions(-1,
		progressbar.OptionSetDescription("Syncing"),
		progressbar.OptionSpinnerType(14),
	)

	if analyticsSyncAll {
		bar.Describe("Syncing all data")
		result, err = syncer.SyncAll(ctx)
	} else if analyticsSyncDays > 0 {
		bar.Describe(fmt.Sprintf("Syncing last %d days", analyticsSyncDays))
		result, err = syncer.SyncRecent(ctx, analyticsSyncDays)
	} else {
		bar.Describe("Incremental sync")
		result, err = syncer.SyncIncremental(ctx)
	}

	bar.Finish()

	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Show results
	duration := result.EndTime.Sub(result.StartTime)
	color.Green("\n✓ Synced %d records in %s", result.RecordsSynced, duration.Round(time.Second))

	if len(result.Errors) > 0 {
		color.Yellow("  Errors: %d", len(result.Errors))
		for _, e := range result.Errors[:min(5, len(result.Errors))] {
			fmt.Printf("    • %s\n", e)
		}
	}

	// Get stats after sync
	statsAfter, _ := syncer.GetSyncStats(ctx)
	fmt.Printf("\nClickHouse records: %d (was %d)\n", statsAfter.TotalCHRecords, statsBefore.TotalCHRecords)

	return nil
}

func runAnalyticsInit(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Connect to ClickHouse
	client, err := getClickHouseClient()
	if err != nil {
		return err
	}

	fmt.Println("Connecting to ClickHouse...")
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	color.Green("✓ Connected")

	fmt.Println("Creating schema...")
	if err := client.InitSchema(ctx); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	color.Green("✓ Schema created")

	// Show table info
	tables, err := client.GetTableInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get table info: %w", err)
	}

	fmt.Println("\nCreated tables:")
	for _, t := range tables {
		fmt.Printf("  • %s (%s)\n", t.Name, t.Engine)
	}

	color.Green("\n✓ Analytics database initialized")

	return nil
}
