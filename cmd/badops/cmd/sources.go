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
	"github.com/badno/badops/internal/source/shopify"
	"github.com/badno/badops/internal/source/tiger"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Manage data sources",
	Long:  `List, test, and get information about available data sources.`,
}

var sourcesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available sources",
	Long:  `Display all registered source connectors.`,
	RunE:  runSourcesList,
}

var sourcesTestCmd = &cobra.Command{
	Use:   "test [source]",
	Short: "Test connection to a source",
	Long:  `Test connectivity and credentials for a specific source or all sources.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSourcesTest,
}

var sourcesInfoCmd = &cobra.Command{
	Use:   "info [source]",
	Short: "Show source information",
	Long:  `Display detailed information about a specific source.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runSourcesInfo,
}

func init() {
	sourcesCmd.AddCommand(sourcesListCmd)
	sourcesCmd.AddCommand(sourcesTestCmd)
	sourcesCmd.AddCommand(sourcesInfoCmd)
}

// initSources initializes all source connectors from config
func initSources() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Register Shopify connector
	shopifyConn := shopify.NewConnector(shopify.Config{
		Store:     cfg.Sources.Shopify.Store,
		APIKeyEnv: cfg.Sources.Shopify.APIKeyEnv,
	})
	source.Register(shopifyConn)

	// Register NOBB connector
	nobbConn := nobb.NewConnector(nobb.Config{
		UsernameEnv: cfg.Sources.NOBB.UsernameEnv,
		PasswordEnv: cfg.Sources.NOBB.PasswordEnv,
	})
	source.Register(nobbConn)

	// Register Tiger.nl connector
	tigerConn := tiger.NewConnector(tiger.Config{
		RateLimitMs: cfg.Sources.TigerNL.RateLimitMs,
	})
	source.Register(tigerConn)

	return nil
}

func runSourcesList(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)

	header.Println("\n  AVAILABLE DATA SOURCES")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	if err := initSources(); err != nil {
		color.Yellow("  Warning: %v", err)
	}

	connectors := source.List()
	if len(connectors) == 0 {
		color.Yellow("  No sources registered.")
		fmt.Println()
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Type", "Capabilities"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	for _, c := range connectors {
		caps := make([]string, 0, len(c.Capabilities()))
		for _, cap := range c.Capabilities() {
			caps = append(caps, string(cap))
		}

		typeStr := string(c.Type())
		if c.Type() == source.TypeSource {
			typeStr = color.GreenString(typeStr)
		} else {
			typeStr = color.YellowString(typeStr)
		}

		table.Append([]string{c.Name(), typeStr, strings.Join(caps, ", ")})
	}

	table.Render()
	fmt.Println()

	return nil
}

func runSourcesTest(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  TESTING SOURCE CONNECTIONS")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	if err := initSources(); err != nil {
		color.Yellow("  Warning: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get sources to test
	var toTest []source.Connector
	if len(args) > 0 {
		c, err := source.Get(args[0])
		if err != nil {
			color.Red("  Error: %v", err)
			return err
		}
		toTest = []source.Connector{c}
	} else {
		toTest = source.List()
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Source", "Status", "Details"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	passed := 0
	failed := 0

	for _, c := range toTest {
		err := c.Test(ctx)
		if err != nil {
			table.Append([]string{
				c.Name(),
				color.RedString("failed"),
				truncate(err.Error(), 50),
			})
			failed++
		} else {
			table.Append([]string{
				c.Name(),
				color.GreenString("ok"),
				"Connection successful",
			})
			passed++
		}
	}

	table.Render()
	fmt.Println()

	if failed == 0 {
		success.Printf("  ✓ All %d sources passed\n", passed)
	} else {
		color.Yellow("  %d passed, %d failed\n", passed, failed)
	}
	fmt.Println()

	return nil
}

func runSourcesInfo(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)

	if err := initSources(); err != nil {
		color.Yellow("  Warning: %v", err)
	}

	c, err := source.Get(args[0])
	if err != nil {
		color.Red("  Error: %v", err)
		return err
	}

	header.Printf("\n  SOURCE: %s\n", strings.ToUpper(c.Name()))
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	fmt.Printf("  Name: %s\n", c.Name())
	fmt.Printf("  Type: %s\n", c.Type())
	fmt.Printf("  Capabilities:\n")
	for _, cap := range c.Capabilities() {
		fmt.Printf("    - %s\n", cap)
	}
	fmt.Println()

	// Show config info based on source type
	cfg, _ := config.Load()
	switch c.Name() {
	case "shopify":
		fmt.Println("  Configuration:")
		fmt.Printf("    Store: %s.myshopify.com\n", cfg.Sources.Shopify.Store)
		fmt.Printf("    API Key Env: %s\n", cfg.Sources.Shopify.APIKeyEnv)
	case "nobb":
		fmt.Println("  Configuration:")
		fmt.Printf("    Username Env: %s\n", cfg.Sources.NOBB.UsernameEnv)
		fmt.Printf("    Password Env: %s\n", cfg.Sources.NOBB.PasswordEnv)
		fmt.Println("    API: https://export.byggtjeneste.no/api")
	case "tiger_nl":
		fmt.Println("  Configuration:")
		fmt.Printf("    Rate Limit: %dms\n", cfg.Sources.TigerNL.RateLimitMs)
		fmt.Println("    Base URL: https://tiger.nl")
	}
	fmt.Println()

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
