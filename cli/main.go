package main

import (
	"fmt"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/aks"
	"github.com/Azure/aks-flex/cli/internal/config"
	"github.com/Azure/aks-flex/cli/internal/network"
	"github.com/Azure/aks-flex/cli/internal/plugin"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var command = &cobra.Command{
	Use:          "aks-flex-cli",
	SilenceUsage: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("aks-flex-cli %s (commit: %s, built: %s)\n", version, commit, date)
	},
}

func init() {
	command.AddCommand(aks.Command)
	command.AddCommand(config.Command)
	command.AddCommand(network.Command)
	command.AddCommand(plugin.Command)
	command.AddCommand(versionCmd)
}

func main() {
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
