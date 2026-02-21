package aks

import (
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/aks/deploy"
)

var Command = &cobra.Command{
	Use: "aks",
}

func init() {
	Command.AddCommand(deploy.Command)
}
