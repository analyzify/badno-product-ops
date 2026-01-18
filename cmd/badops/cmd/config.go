package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/badno/badops/internal/config"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `Initialize, view, and modify configuration settings.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration file",
	Long:  `Create a new configuration file with default settings.`,
	RunE:  runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display all configuration settings.`,
	RunE:  runConfigShow,
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a configuration value",
	Long:  `Set a specific configuration value.`,
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a configuration value",
	Long:  `Get a specific configuration value.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  INITIALIZING CONFIGURATION")
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	// Check if config already exists
	if config.Exists() {
		configPath, _ := config.GetConfigPath()
		color.Yellow("  Configuration file already exists: %s", configPath)
		fmt.Println()
		return nil
	}

	// Create config
	if err := config.Init(); err != nil {
		color.Red("  Error: %v", err)
		return err
	}

	configPath, _ := config.GetConfigPath()
	success.Printf("  ✓ Created configuration file: %s\n", configPath)
	fmt.Println()

	// Show next steps
	color.Yellow("  Next steps:")
	fmt.Println("    1. Set your Shopify API key:")
	fmt.Println("       export SHOPIFY_API_KEY=your_key_here")
	fmt.Println()
	fmt.Println("    2. Set your NOBB credentials (optional):")
	fmt.Println("       export NOBB_USERNAME=your_username")
	fmt.Println("       export NOBB_PASSWORD=your_password")
	fmt.Println()
	fmt.Println("    3. Customize settings:")
	fmt.Println("       badops config set sources.shopify.store your_store")
	fmt.Println()

	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)

	header.Println("\n  CURRENT CONFIGURATION")
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		color.Red("  Error loading configuration: %v", err)
		return err
	}

	configPath, _ := config.GetConfigPath()
	if config.Exists() {
		color.Yellow("  Config file: %s\n\n", configPath)
	} else {
		color.Yellow("  Using default configuration (no config file)\n\n")
	}

	// Format as YAML
	data, _ := yaml.Marshal(cfg)
	fmt.Println("  " + strings.ReplaceAll(string(data), "\n", "\n  "))
	fmt.Println()

	// Show environment variable status
	header.Println("  ENVIRONMENT VARIABLES")
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Variable", "Status"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	envVars := []struct {
		name     string
		envName  string
	}{
		{"Shopify API Key", cfg.Sources.Shopify.APIKeyEnv},
		{"NOBB Username", cfg.Sources.NOBB.UsernameEnv},
		{"NOBB Password", cfg.Sources.NOBB.PasswordEnv},
	}

	for _, ev := range envVars {
		status := color.RedString("not set")
		if os.Getenv(ev.envName) != "" {
			status = color.GreenString("set")
		}
		table.Append([]string{ev.name + " (" + ev.envName + ")", status})
	}

	table.Render()
	fmt.Println()

	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	if err := config.Set(key, value); err != nil {
		color.Red("  Error: %v", err)
		return err
	}

	color.Green("  ✓ Set %s = %s", key, value)
	fmt.Println()
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	value, err := config.Get(key)
	if err != nil {
		color.Red("  Error: %v", err)
		return err
	}

	fmt.Printf("  %s = %s\n", key, value)
	fmt.Println()
	return nil
}
