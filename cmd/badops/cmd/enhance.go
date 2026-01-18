package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/badno/badops/internal/config"
	"github.com/badno/badops/internal/source"
	"github.com/badno/badops/internal/source/nobb"
	"github.com/badno/badops/internal/source/tiger"
	"github.com/badno/badops/internal/state"
	"github.com/badno/badops/pkg/models"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	enhanceSources  []string
	enhanceLimit    int
	enhanceDryRun   bool
	enhanceVendor   string
)

var enhanceCmd = &cobra.Command{
	Use:   "enhance",
	Short: "Enhance products with additional data",
	Long:  `Run enhancement pipelines to enrich products with data from various sources.`,
}

var enhanceRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run enhancements on products",
	Long:  `Enhance products using specified sources (nobb, tiger_nl).`,
	RunE:  runEnhance,
}

var enhanceReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Review pending enhancements",
	Long:  `Show products with pending or unapplied enhancements.`,
	RunE:  runEnhanceReview,
}

var enhanceApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply approved enhancements",
	Long:  `Apply enhancements that have been reviewed and approved.`,
	RunE:  runEnhanceApply,
}

func init() {
	enhanceRunCmd.Flags().StringSliceVar(&enhanceSources, "source", []string{"tiger_nl"}, "Enhancement sources to use (tiger_nl, nobb)")
	enhanceRunCmd.Flags().IntVar(&enhanceLimit, "limit", 0, "Maximum products to enhance (0 = all)")
	enhanceRunCmd.Flags().BoolVar(&enhanceDryRun, "dry-run", false, "Preview without making changes")
	enhanceRunCmd.Flags().StringVar(&enhanceVendor, "vendor", "", "Only enhance products from this vendor")

	enhanceCmd.AddCommand(enhanceRunCmd)
	enhanceCmd.AddCommand(enhanceReviewCmd)
	enhanceCmd.AddCommand(enhanceApplyCmd)
}

func runEnhance(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  ENHANCING PRODUCTS")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	// Load state
	store := state.NewStore("")
	if err := store.Load(); err != nil {
		color.Red("  Error loading state: %v", err)
		return err
	}

	// Get products to enhance
	products := store.GetAllProducts()
	if len(products) == 0 {
		color.Yellow("  No products found. Run 'badops products import' or 'badops products parse' first.")
		return nil
	}

	// Filter by vendor if specified
	if enhanceVendor != "" {
		filtered := make([]*models.EnhancedProduct, 0)
		for _, p := range products {
			if strings.EqualFold(p.Vendor, enhanceVendor) {
				filtered = append(filtered, p)
			}
		}
		products = filtered
	}

	// Apply limit
	if enhanceLimit > 0 && enhanceLimit < len(products) {
		products = products[:enhanceLimit]
	}

	color.Yellow("  Found %d products to enhance\n", len(products))
	color.Yellow("  Sources: %s\n", strings.Join(enhanceSources, ", "))
	if enhanceDryRun {
		color.Yellow("  Mode: DRY RUN (no changes will be made)\n")
	}
	fmt.Println()

	// Load config and initialize enhancers
	cfg, err := config.Load()
	if err != nil {
		color.Yellow("  Warning: Could not load config, using defaults")
		cfg = config.DefaultConfig()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Initialize connectors based on requested sources
	enhancers := make(map[string]source.Connector)
	for _, src := range enhanceSources {
		switch src {
		case "tiger_nl":
			conn := tiger.NewConnector(tiger.Config{
				RateLimitMs: cfg.Sources.TigerNL.RateLimitMs,
			})
			if err := conn.Connect(ctx); err != nil {
				color.Yellow("  Warning: Could not connect to Tiger.nl: %v", err)
				continue
			}
			enhancers[src] = conn
		case "nobb":
			conn := nobb.NewConnector(nobb.Config{
				UsernameEnv: cfg.Sources.NOBB.UsernameEnv,
				PasswordEnv: cfg.Sources.NOBB.PasswordEnv,
			})
			if err := conn.Connect(ctx); err != nil {
				color.Yellow("  Warning: Could not connect to NOBB: %v", err)
				continue
			}
			enhancers[src] = conn
		default:
			color.Yellow("  Warning: Unknown source: %s", src)
		}
	}

	if len(enhancers) == 0 {
		color.Red("  Error: No enhancement sources available")
		return fmt.Errorf("no enhancement sources available")
	}

	// Progress bar
	bar := progressbar.NewOptions(len(products),
		progressbar.OptionSetDescription("  Enhancing products"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        color.GreenString("█"),
			SaucerHead:    color.GreenString("█"),
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
	)

	// Enhance each product
	results := make([]struct {
		sku      string
		source   string
		status   string
		details  string
	}, 0)

	enhanced := 0
	imagesAdded := 0
	fieldsAdded := 0

	for _, p := range products {
		bar.Add(1)

		for srcName, enhancer := range enhancers {
			if enhanceDryRun {
				results = append(results, struct {
					sku, source, status, details string
				}{p.SKU, srcName, "dry-run", "Would enhance"})
				continue
			}

			result, err := enhancer.EnhanceProduct(ctx, p)
			if err != nil {
				results = append(results, struct {
					sku, source, status, details string
				}{p.SKU, srcName, "error", err.Error()})
				continue
			}

			if result.Success {
				enhanced++
				imagesAdded += result.ImagesAdded
				fieldsAdded += len(result.FieldsUpdated)

				details := ""
				if result.ImagesAdded > 0 {
					details = fmt.Sprintf("+%d images", result.ImagesAdded)
				}
				if len(result.FieldsUpdated) > 0 {
					if details != "" {
						details += ", "
					}
					details += fmt.Sprintf("+%d fields", len(result.FieldsUpdated))
				}
				if details == "" {
					details = "no changes"
				}

				results = append(results, struct {
					sku, source, status, details string
				}{p.SKU, srcName, "ok", details})
			} else {
				errMsg := "failed"
				if result.Error != nil {
					errMsg = result.Error.Error()
				}
				results = append(results, struct {
					sku, source, status, details string
				}{p.SKU, srcName, "failed", truncate(errMsg, 30)})
			}
		}
	}

	fmt.Println()
	fmt.Println()

	// Show results table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "Source", "Status", "Details"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	// Only show first 20 results to avoid overwhelming output
	displayCount := len(results)
	if displayCount > 20 {
		displayCount = 20
	}

	for i := 0; i < displayCount; i++ {
		r := results[i]
		statusColor := color.GreenString(r.status)
		if r.status == "error" || r.status == "failed" {
			statusColor = color.RedString(r.status)
		} else if r.status == "dry-run" {
			statusColor = color.YellowString(r.status)
		}
		table.Append([]string{r.sku, r.source, statusColor, r.details})
	}

	if len(results) > 20 {
		table.Append([]string{"...", "...", "...", fmt.Sprintf("and %d more", len(results)-20)})
	}

	table.Render()
	fmt.Println()

	// Summary
	if !enhanceDryRun {
		success.Printf("  ✓ Enhanced %d products\n", enhanced)
		if imagesAdded > 0 {
			success.Printf("  ✓ Added %d images\n", imagesAdded)
		}
		if fieldsAdded > 0 {
			success.Printf("  ✓ Updated %d fields\n", fieldsAdded)
		}

		// Save state
		store.AddHistory("enhance", strings.Join(enhanceSources, ","), enhanced,
			fmt.Sprintf("Enhanced %d products with %d images", enhanced, imagesAdded))
		if err := store.Save(); err != nil {
			color.Red("  Warning: Could not save state: %v", err)
		} else {
			success.Println("  ✓ State saved")
		}
	} else {
		color.Yellow("  Dry run complete. No changes made.")
	}
	fmt.Println()

	return nil
}

func runEnhanceReview(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)

	header.Println("\n  REVIEWING ENHANCEMENTS")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	// Load state
	store := state.NewStore("")
	if err := store.Load(); err != nil {
		color.Red("  Error loading state: %v", err)
		return err
	}

	// Get products with enhancements
	products := store.GetAllProducts()
	enhancedProducts := make([]*models.EnhancedProduct, 0)
	for _, p := range products {
		if len(p.Enhancements) > 0 {
			enhancedProducts = append(enhancedProducts, p)
		}
	}

	if len(enhancedProducts) == 0 {
		color.Yellow("  No enhanced products found.")
		fmt.Println()
		return nil
	}

	color.Yellow("  Found %d products with enhancements\n\n", len(enhancedProducts))

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "Title", "Enhancements", "Images", "Status"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	for _, p := range enhancedProducts {
		title := p.Title
		if len(title) > 25 {
			title = title[:22] + "..."
		}

		// Count enhancements by source
		sources := make(map[string]int)
		for _, e := range p.Enhancements {
			sources[e.Source]++
		}
		enhList := make([]string, 0)
		for src, count := range sources {
			enhList = append(enhList, fmt.Sprintf("%s:%d", src, count))
		}

		// Count images by source
		imgCount := 0
		newImages := 0
		for _, img := range p.Images {
			imgCount++
			if img.Source != "shopify" {
				newImages++
			}
		}
		imgStr := fmt.Sprintf("%d (+%d)", imgCount, newImages)

		statusStr := color.YellowString(string(p.Status))
		if p.Status == models.StatusApproved {
			statusStr = color.GreenString(string(p.Status))
		}

		table.Append([]string{p.SKU, title, strings.Join(enhList, ", "), imgStr, statusStr})
	}

	table.Render()
	fmt.Println()

	color.Yellow("  To approve enhancements: badops enhance apply")
	fmt.Println()

	return nil
}

func runEnhanceApply(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  APPLYING ENHANCEMENTS")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	// Load state
	store := state.NewStore("")
	if err := store.Load(); err != nil {
		color.Red("  Error loading state: %v", err)
		return err
	}

	// Get products with pending enhancements
	products := store.GetAllProducts()
	applied := 0

	for _, p := range products {
		if len(p.Enhancements) > 0 && p.Status != models.StatusApproved {
			p.Status = models.StatusApproved
			store.SetProduct(p)
			applied++
		}
	}

	if applied == 0 {
		color.Yellow("  No pending enhancements to apply.")
		fmt.Println()
		return nil
	}

	// Save state
	store.AddHistory("apply", "manual", applied,
		fmt.Sprintf("Approved %d products", applied))
	if err := store.Save(); err != nil {
		color.Red("  Error saving state: %v", err)
		return err
	}

	success.Printf("  ✓ Applied enhancements to %d products\n", applied)
	color.Yellow("  → Run 'badops export run' to export enhanced products")
	fmt.Println()

	return nil
}
