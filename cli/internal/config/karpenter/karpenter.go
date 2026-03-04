package karpenter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
	"github.com/Azure/aks-flex/plugin/pkg/util/az"
	"github.com/Azure/aks-flex/plugin/pkg/util/ssh"
)

var helmTemplate = template.Must(template.New("karpenter-helm").Parse(`helm upgrade --install karpenter charts/karpenter \
  --namespace karpenter \
  --create-namespace \
  --set settings.clusterName="{{ .ClusterName }}" \
  --set settings.clusterEndpoint="{{ .ClusterEndpoint }}" \
  --set logLevel=debug \
  --set replicas=1 \
  --set controller.nebiusCredentials.enabled=true \
  --set controller.image.digest="" \
{{- if .ImageRepository }}
  --set controller.image.repository="{{ .ImageRepository }}" \
{{- end }}
{{- if .ImageTag }}
  --set controller.image.tag="{{ .ImageTag }}" \
{{- end }}
  --set "serviceAccount.annotations.azure\.workload\.identity/client-id={{ .AzureClientID }}" \
  --set-string "podLabels.azure\.workload\.identity/use=true" \
  --set "controller.env[0].name=ARM_CLOUD,controller.env[0].value={{ .ARMCloud }}" \
  --set "controller.env[1].name=LOCATION,controller.env[1].value={{ .Location }}" \
  --set "controller.env[2].name=ARM_RESOURCE_GROUP,controller.env[2].value={{ .ARMResourceGroup }}" \
  --set "controller.env[3].name=AZURE_TENANT_ID,controller.env[3].value={{ .AzureTenantID }}" \
  --set "controller.env[4].name=AZURE_CLIENT_ID,controller.env[4].value={{ .AzureClientID }}" \
  --set "controller.env[5].name=AZURE_SUBSCRIPTION_ID,controller.env[5].value={{ .AzureSubscriptionID }}" \
  --set "controller.env[6].name=AZURE_NODE_RESOURCE_GROUP,controller.env[6].value={{ .NodeResourceGroup }}" \
  --set "controller.env[7].name=SSH_PUBLIC_KEY,controller.env[7].value={{ .SSHPublicKey }}" \
  --set "controller.env[8].name=VNET_SUBNET_ID,controller.env[8].value={{ .VNETSubnetID }}" \
  --set "controller.env[9].name=KUBELET_BOOTSTRAP_TOKEN,controller.env[9].value={{ .KubeletBootstrapToken }}" \
  --set-string "controller.env[10].name=DISABLE_LEADER_ELECTION,controller.env[10].value=false"
`))

type helmContext struct {
	ClusterName           string
	ClusterEndpoint       string
	ARMCloud              string
	Location              string
	ARMResourceGroup      string
	AzureTenantID         string
	AzureClientID         string
	AzureSubscriptionID   string
	NodeResourceGroup     string
	SSHPublicKey          string
	VNETSubnetID          string
	KubeletBootstrapToken string
	ImageRepository       string
	ImageTag              string
}

func (hc *helmContext) resolve(ctx context.Context) {
	cfg := configcmd.DefaultConfig()

	// Values sourced from the shared config.
	clusterName := ""
	location := ""
	resourceGroup := ""
	subscriptionID := ""
	if cfg != nil {
		clusterName = cfg.ClusterName
		location = cfg.Location
		resourceGroup = cfg.ResourceGroupName
		subscriptionID = cfg.SubscriptionID
	}

	hc.ClusterName = configcmd.OrPlaceholder(clusterName)
	hc.Location = configcmd.OrPlaceholder(location)
	hc.ARMResourceGroup = configcmd.OrPlaceholder(resourceGroup)
	hc.AzureSubscriptionID = configcmd.OrPlaceholder(subscriptionID)
	hc.ARMCloud = "AzurePublicCloud"

	// Tenant ID — resolve via az CLI.
	if tenantID, err := execOutput(ctx, "az", "account", "show", "--query", "tenantId", "-o", "tsv"); err == nil {
		hc.AzureTenantID = tenantID
	} else {
		hc.AzureTenantID = configcmd.OrPlaceholder("")
	}

	// SSH public key — resolve from local SSH key files.
	if pubKey, err := ssh.PublicKey(); err == nil {
		hc.SSHPublicKey = strings.TrimSpace(string(pubKey))
	} else {
		hc.SSHPublicKey = configcmd.OrPlaceholder("")
	}

	// Bootstrap token — resolve from DefaultKubeadmConfig.
	kubeadmCfg := configcmd.DefaultKubeadmConfig(ctx)
	hc.KubeletBootstrapToken = configcmd.OrPlaceholder(kubeadmCfg.GetToken())

	// Cluster endpoint, node resource group, and VNET subnet ID — resolve
	// from the Azure ManagedCluster resource.
	hc.ClusterEndpoint = configcmd.OrPlaceholder("")
	hc.NodeResourceGroup = configcmd.OrPlaceholder("")
	hc.VNETSubnetID = configcmd.OrPlaceholder("")

	// Karpenter managed identity client ID — resolve via az CLI.
	if clientID, err := execOutput(ctx, "az", "identity", "show",
		"--name", "karpenter-flex",
		"--resource-group", resourceGroup,
		"--query", "clientId", "-o", "tsv"); err == nil {
		hc.AzureClientID = clientID
	} else {
		hc.AzureClientID = configcmd.OrPlaceholder("")
	}

	if cfg == nil {
		return
	}

	credentials, err := azidentity.NewAzureCLICredential(nil)
	if err != nil {
		warn("could not obtain Azure credentials: %v", err)
		return
	}

	mc, err := az.ManagedCluster(ctx, credentials, cfg)
	if err != nil {
		warn("could not retrieve managed cluster: %v", err)
		return
	}

	if mc.Properties != nil {
		if mc.Properties.Fqdn != nil {
			hc.ClusterEndpoint = "https://" + *mc.Properties.Fqdn + ":443"
		}
		if mc.Properties.NodeResourceGroup != nil {
			hc.NodeResourceGroup = *mc.Properties.NodeResourceGroup
		}
		if len(mc.Properties.AgentPoolProfiles) > 0 && mc.Properties.AgentPoolProfiles[0].VnetSubnetID != nil {
			hc.VNETSubnetID = *mc.Properties.AgentPoolProfiles[0].VnetSubnetID
		}
	}
}

func execOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
	fmt.Fprintln(os.Stderr, "Using placeholder values — edit the output before applying.")
}

// Command is the "config karpenter" parent command.
var Command = &cobra.Command{
	Use:   "karpenter",
	Short: "Karpenter-provider-flex configuration commands",
}

var flagImage string

var helmCmd = &cobra.Command{
	Use:   "helm",
	Short: "Print helm install/upgrade command for karpenter",
	Long: `Print a helm upgrade --install command for deploying karpenter.

Values are resolved from the shared config (environment variables) and the
Azure ManagedCluster resource. Any value that cannot be resolved is replaced
with a placeholder that must be edited before running the command.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runHelm(cmd.Context(), cmd.OutOrStdout())
	},
}

func init() {
	helmCmd.Flags().StringVar(&flagImage, "image", "", "override controller image (e.g. myregistry.io/karpenter:v0.1.0)")
	Command.AddCommand(helmCmd)
}

func runHelm(ctx context.Context, out io.Writer) error {
	hc := &helmContext{}
	hc.resolve(ctx)

	// Parse --image flag into repository and tag if provided.
	if flagImage != "" {
		if repo, tag, ok := strings.Cut(flagImage, ":"); ok {
			hc.ImageRepository = repo
			hc.ImageTag = tag
		} else {
			hc.ImageRepository = flagImage
		}
	}

	var buf bytes.Buffer
	if err := helmTemplate.Execute(&buf, hc); err != nil {
		return fmt.Errorf("rendering helm command: %w", err)
	}

	_, err := io.Copy(out, &buf)
	return err
}
