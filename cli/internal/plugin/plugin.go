package plugin

import (
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/plugin/apply"
	"github.com/Azure/aks-flex/cli/internal/plugin/delete"
	"github.com/Azure/aks-flex/cli/internal/plugin/get"
)

var Command = &cobra.Command{
	Use: "plugin",
}

func init() {
	Command.AddCommand(get.Command)
	Command.AddCommand(apply.Command)
	Command.AddCommand(delete.Command)
}
