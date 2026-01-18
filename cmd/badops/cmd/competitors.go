package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/badno/badops/internal/database"
	"github.com/badno/badops/internal/database/postgres"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var competitorsCmd = &cobra.Command{
	Use:   "competitors",
	Short: "Competitor management commands",
	Long:  "Commands for managing competitor data and tracking",
}

var competitorsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all competitors",
	Long:  "Shows all tracked competitors with their product counts",
	RunE:  runCompetitorsList,
}

var competitorsAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new competitor",
	Long:  "Adds a new competitor to track",
	Args:  cobra.ExactArgs(1),
	RunE:  runCompetitorsAdd,
}

var competitorsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show competitor statistics",
	Long:  "Shows detailed statistics about competitor coverage",
	RunE:  runCompetitorsStats,
}

var competitorsRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a competitor",
	Long:  "Removes a competitor and all associated data",
	Args:  cobra.ExactArgs(1),
	RunE:  runCompetitorsRemove,
}

var (
	competitorWebsite string
)

func init() {
	competitorsCmd.AddCommand(competitorsListCmd)
	competitorsCmd.AddCommand(competitorsAddCmd)
	competitorsCmd.AddCommand(competitorsStatsCmd)
	competitorsCmd.AddCommand(competitorsRemoveCmd)

	competitorsAddCmd.Flags().StringVar(&competitorWebsite, "website", "", "Competitor website URL")
}

func runCompetitorsList(cmd *cobra.Command, args []string) error {
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

	// Get competitors
	repo := postgres.NewCompetitorRepo(client)
	competitors, err := repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get competitors: %w", err)
	}

	if len(competitors) == 0 {
		color.Yellow("No competitors found")
		fmt.Println("\nAdd competitors using:")
		fmt.Println("  badops competitors add \"Competitor Name\"")
		fmt.Println("Or import from a Reprice CSV:")
		fmt.Println("  badops prices import reprice-export.csv")
		return nil
	}

	fmt.Printf("Found %d competitors:\n\n", len(competitors))

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Name", "Products", "Website", "Last Scraped"})
	table.SetBorder(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, c := range competitors {
		lastScraped := "-"
		if c.LastScraped != nil {
			lastScraped = c.LastScraped.Format("2006-01-02")
		}

		website := "-"
		if c.Website != "" {
			website = truncateString(c.Website, 30)
		}

		table.Append([]string{
			fmt.Sprintf("%d", c.ID),
			c.Name,
			fmt.Sprintf("%d", c.ProductCount),
			website,
			lastScraped,
		})
	}

	table.Render()

	return nil
}

func runCompetitorsAdd(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := args[0]

	// Connect to database
	client, err := getDBClient()
	if err != nil {
		return err
	}

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	// Check if competitor already exists
	repo := postgres.NewCompetitorRepo(client)
	existing, err := repo.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check existing: %w", err)
	}
	if existing != nil {
		color.Yellow("Competitor '%s' already exists (ID: %d)", name, existing.ID)
		return nil
	}

	// Create competitor
	competitor := &database.Competitor{
		Name:          name,
		Website:       competitorWebsite,
		ScrapeEnabled: false,
	}

	if err := repo.Create(ctx, competitor); err != nil {
		return fmt.Errorf("failed to create competitor: %w", err)
	}

	color.Green("✓ Added competitor: %s (ID: %d)", name, competitor.ID)

	return nil
}

func runCompetitorsStats(cmd *cobra.Command, args []string) error {
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

	// Get competitors
	competitorRepo := postgres.NewCompetitorRepo(client)
	competitors, err := competitorRepo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get competitors: %w", err)
	}

	// Get product count
	productRepo := postgres.NewProductRepo(client)
	totalProducts, err := productRepo.Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to count products: %w", err)
	}

	// Get link stats
	linkRepo := postgres.NewCompetitorProductRepo(client)
	totalLinks, err := linkRepo.Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to count links: %w", err)
	}

	linkCounts, err := linkRepo.CountByCompetitor(ctx)
	if err != nil {
		return fmt.Errorf("failed to get link counts: %w", err)
	}

	// Get price observation count
	priceRepo := postgres.NewPriceObservationRepo(client)
	totalObs, err := priceRepo.Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to count observations: %w", err)
	}

	// Summary
	fmt.Println(color.CyanString("Competitor Statistics"))
	fmt.Printf("  Total Competitors:   %d\n", len(competitors))
	fmt.Printf("  Total Products:      %d\n", totalProducts)
	fmt.Printf("  Total Product Links: %d\n", totalLinks)
	fmt.Printf("  Price Observations:  %d\n", totalObs)

	if totalProducts > 0 {
		coverage := float64(totalLinks) / float64(totalProducts) / float64(len(competitors)) * 100
		if len(competitors) > 0 {
			fmt.Printf("  Average Coverage:    %.1f%%\n", coverage)
		}
	}

	// Per-competitor stats
	if len(competitors) > 0 {
		fmt.Println("\n" + color.CyanString("Coverage by Competitor"))

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Competitor", "Products", "Coverage", "Status"})
		table.SetBorder(false)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)

		for _, c := range competitors {
			productCount := linkCounts[c.ID]
			coverage := float64(0)
			if totalProducts > 0 {
				coverage = float64(productCount) / float64(totalProducts) * 100
			}

			status := color.GreenString("Active")
			if c.LastScraped == nil {
				status = color.YellowString("No Data")
			} else if time.Since(*c.LastScraped) > 7*24*time.Hour {
				status = color.YellowString("Stale")
			}

			table.Append([]string{
				c.Name,
				fmt.Sprintf("%d", productCount),
				fmt.Sprintf("%.1f%%", coverage),
				status,
			})
		}

		table.Render()
	}

	// Coverage distribution
	if totalProducts > 0 && len(competitors) > 0 {
		fmt.Println("\n" + color.CyanString("Product Coverage Distribution"))

		// Query to get products by number of competitors tracking them
		query := `
			SELECT competitor_count, COUNT(*) as product_count
			FROM (
				SELECT p.id, COUNT(cp.competitor_id) as competitor_count
				FROM products p
				LEFT JOIN competitor_products cp ON p.id = cp.product_id
				GROUP BY p.id
			) subq
			GROUP BY competitor_count
			ORDER BY competitor_count
		`

		rows, err := client.Pool().Query(ctx, query)
		if err == nil {
			defer rows.Close()

			var distributions []struct {
				competitors int
				products    int
			}

			for rows.Next() {
				var d struct {
					competitors int
					products    int
				}
				if err := rows.Scan(&d.competitors, &d.products); err == nil {
					distributions = append(distributions, d)
				}
			}

			for _, d := range distributions {
				pct := float64(d.products) / float64(totalProducts) * 100
				label := fmt.Sprintf("%d competitors", d.competitors)
				if d.competitors == 0 {
					label = "No coverage"
				} else if d.competitors == 1 {
					label = "1 competitor"
				}
				fmt.Printf("  %s: %d products (%.1f%%)\n", label, d.products, pct)
			}
		}
	}

	return nil
}

func runCompetitorsRemove(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := args[0]

	// Connect to database
	client, err := getDBClient()
	if err != nil {
		return err
	}

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	// Find competitor
	repo := postgres.NewCompetitorRepo(client)
	competitor, err := repo.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to find competitor: %w", err)
	}
	if competitor == nil {
		return fmt.Errorf("competitor '%s' not found", name)
	}

	// Show what will be deleted
	fmt.Printf("This will remove:\n")
	fmt.Printf("  • Competitor: %s\n", competitor.Name)
	fmt.Printf("  • %d product links\n", competitor.ProductCount)
	fmt.Printf("  • All associated price observations\n")
	fmt.Println()

	// Confirm deletion
	fmt.Print("Are you sure? [y/N]: ")
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "y" && confirm != "Y" {
		fmt.Println("Cancelled")
		return nil
	}

	// Delete competitor (cascades to related tables)
	if err := repo.Delete(ctx, competitor.ID); err != nil {
		return fmt.Errorf("failed to delete competitor: %w", err)
	}

	color.Green("✓ Removed competitor: %s", name)

	return nil
}

// Helper functions
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
