package main

import (
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/aks"
	"github.com/Azure/aks-flex/cli/internal/config"
	"github.com/Azure/aks-flex/cli/internal/network"
	"github.com/Azure/aks-flex/cli/internal/plugin"
)

var command = &cobra.Command{
	Use:          "flex-cli",
	SilenceUsage: true,
}

func init() {
	command.AddCommand(aks.Command)
	command.AddCommand(config.Command)
	command.AddCommand(network.Command)
	command.AddCommand(plugin.Command)
}

func main() {
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
