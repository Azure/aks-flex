package env

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"os/user"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

var configTemplate = template.Must(template.New("env").Parse(`
# -----------------------------------------------------------------------------
# Azure Config
# -----------------------------------------------------------------------------
# Azure side resource location
export LOCATION={{.AzureLocation}}
# Azure side subscription ID
export AZURE_SUBSCRIPTION_ID={{.AzureSubscriptionID}}
# Azure side resource group name
export RESOURCE_GROUP_NAME={{.AzureResourceGroupName}}

# -----------------------------------------------------------------------------
# Remote Clouds Config
# -----------------------------------------------------------------------------
{{- if .EnableNebius }}
# Nebius project ID
export NEBIUS_PROJECT_ID={{.NebiusProjectID}}
# Nebius region
export NEBIUS_REGION=<update with your Nebius region>
# Nebius credential file path
# ref: https://docs.nebius.com/iam/service-accounts/authorized-keys
export NEBIUS_CREDENTIAL_FILE=<update with your Nebius credential file path>
{{- end }}
`))

type configContext struct {
	AzureLocation          string
	AzureSubscriptionID    string
	AzureResourceGroupName string

	EnableNebius         bool
	NebiusProjectID      string
	NebiusRegion         string
	NebiusCredentialFile string
}

func execAndGetOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- input command is defined by us, not user input
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (cc *configContext) ResolveAzureConfig(ctx context.Context) error {
	if v, err := execAndGetOutput(ctx, "az", "account", "show", "--query", "id", "-o", "tsv"); err == nil {
		cc.AzureSubscriptionID = strings.TrimSpace(v)
	}
	cc.AzureLocation = "southcentralus"

	if currentUser, err := user.Current(); err == nil {
		cc.AzureResourceGroupName = "rg-aks-flex-" + strings.ToLower(currentUser.Username)
	} else {
		// fallback to timestamp based resource group name to avoid collision
		cc.AzureResourceGroupName = fmt.Sprintf("rg-aks-flex-%s", time.Now().Format("0102-1504"))
	}

	return nil
}

func (cc *configContext) ResolveNebiusConfig(ctx context.Context) error {
	cc.EnableNebius = flagEnableNebius
	if !cc.EnableNebius {
		return nil
	}

	if v, err := execAndGetOutput(ctx, "nebius", "config", "get", "parent-id"); err == nil {
		cc.NebiusProjectID = strings.TrimSpace(v)
	} else {
		cc.NebiusProjectID = "<update with your Nebius project ID>"
	}

	return nil
}

var (
	flagEnableNebius bool
)

func init() {
	Command.Flags().BoolVar(&flagEnableNebius, "nebius", flagEnableNebius, "include Nebius related environment variables")
}

var Command = &cobra.Command{
	Use:   "env",
	Short: "Print default environment configs",
	RunE: func(cmd *cobra.Command, args []string) error {
		return run(cmd.Context(), cmd.OutOrStdout())
	},
}

func run(ctx context.Context, out io.Writer) error {
	cc := &configContext{}

	if err := cc.ResolveAzureConfig(ctx); err != nil {
		return err
	}

	if err := cc.ResolveNebiusConfig(ctx); err != nil {
		return err
	}

	return configTemplate.Execute(out, cc)
}
