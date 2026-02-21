package network

import (
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/network/deploy"
)

var Command = &cobra.Command{
	Use: "network",
}

func init() {
	Command.AddCommand(deploy.Command)
}
