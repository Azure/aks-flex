package config

import (
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/config/agentpools"
	"github.com/Azure/aks-flex/cli/internal/config/env"
	"github.com/Azure/aks-flex/cli/internal/config/network"
	"github.com/Azure/aks-flex/cli/internal/config/nodebootstrap"
)

var Command = &cobra.Command{
	Use: "config",
}

func init() {
	Command.AddCommand(env.Command)
	Command.AddCommand(network.Command)
	Command.AddCommand(agentpools.Command)
	Command.AddCommand(nodebootstrap.Command)
}
