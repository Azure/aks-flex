package agentpools

import (
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
)

var r = configcmd.NewRouter("agentpools", "Generate a default agent pool config for a remote cloud")

func init() {
	r.Handle("aws", configcmd.ProtoHandler(newAWSAgentPool))
	r.Handle("azure", configcmd.ProtoHandler(newAzureAgentPool))
	r.Handle("nebius", configcmd.ProtoHandler(newNebiusAgentPool))
}

var Command *cobra.Command = r.Command()
