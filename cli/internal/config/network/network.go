package network

import (
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
)

var r = configcmd.NewRouter("network", "Generate a default network config for a remote cloud")

func init() {
	r.Handle("aws", configcmd.ProtoHandler(newAWSNetwork))
}

var Command *cobra.Command = r.Command()
