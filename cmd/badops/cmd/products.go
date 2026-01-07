package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/badno/badops/internal/matcher"
	"github.com/badno/badops/internal/parser"
	"github.com/badno/badops/pkg/models"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	parsedProducts []models.Product
	stateFile      = "output/.badops-state.json"
)

var productsCmd = &cobra.Command{
	Use:   "products",
	Short: "Manage product data",
	Long:  `Parse, match, and manage product data from Matrixify exports.`,
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

func init() {
	productsCmd.AddCommand(parseCmd)
	productsCmd.AddCommand(matchCmd)
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
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(products, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0644)
}

func loadState() ([]models.Product, error) {
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
