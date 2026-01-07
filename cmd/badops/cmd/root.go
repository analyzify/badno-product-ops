package cmd

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "badops",
	Short: "Bad.no Operations Terminal",
	Long: color.New(color.FgCyan, color.Bold).Sprint(`
  ____            _
 | __ )  __ _  __| |  ___  _ __  ___
 |  _ \ / _' |/ _' | / _ \| '_ \/ __|
 | |_) | (_| | (_| || (_) | |_) \__ \
 |____/ \__,_|\__,_| \___/| .__/|___/
                          |_|
`) + `
Bad.no Operations Terminal - Product image enhancement toolkit

Streamline your product operations with automated image fetching,
matching, and processing from supplier catalogs.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(productsCmd)
	rootCmd.AddCommand(imagesCmd)
}
