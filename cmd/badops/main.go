package main

import (
	"os"

	"github.com/badno/badops/cmd/badops/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
