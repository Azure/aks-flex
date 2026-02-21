package config

import (
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/config/env"
)

var Command = &cobra.Command{
	Use: "config",
}

func init() {
	Command.AddCommand(env.Command)
}
