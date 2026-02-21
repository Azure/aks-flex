package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/aks"
	"github.com/Azure/aks-flex/cli/internal/config"
	"github.com/Azure/aks-flex/cli/internal/network"
)

var command = &cobra.Command{
	Use:          "flex-cli",
	SilenceUsage: true,
}

func init() {
	command.AddCommand(aks.Command)
	command.AddCommand(config.Command)
	command.AddCommand(network.Command)
}

func main() {
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
