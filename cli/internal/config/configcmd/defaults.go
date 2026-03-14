package configcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/kubeadm"
	"github.com/Azure/aks-flex/plugin/pkg/util/config"
	kubeadmutil "github.com/Azure/aks-flex/plugin/pkg/util/kubeadm"
)

const placeholder = "<replace-with-actual-value>"

// DefaultConfig attempts to load the shared [config.Config] from environment
// variables. If validation fails (e.g. Azure env vars not set) it returns nil
// and prints a warning to stderr.
func DefaultConfig() *config.Config {
	cfg, err := config.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config from environment: %v\n", err)
		fmt.Fprintln(os.Stderr, "Ensure your .env file is sourced: source .env")
		fmt.Fprintln(os.Stderr, "Using placeholder values — edit the output before applying.")
		return nil
	}
	return cfg
}

// OrPlaceholder returns val if non-empty, otherwise returns the placeholder sentinel.
func OrPlaceholder(val string) string {
	if val != "" {
		return val
	}
	return placeholder
}

// DefaultKubeadmConfig attempts to retrieve kubeadm configuration from the
// running AKS cluster via [kubeadmutil.FromAKS]. If the cluster is not
// reachable or the required environment variables are not set, it falls back
// to placeholder values that the user must replace manually.
func DefaultKubeadmConfig(ctx context.Context) *kubeadm.Config {
	credOptions := &azidentity.AzureCLICredentialOptions{}
	if tenantID := azureConfigTenantID(); tenantID != "" {
		credOptions.TenantID = tenantID
	}
	credentials, err := azidentity.NewAzureCLICredential(credOptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not obtain Azure CLI credentials: %v\n", err)
		fmt.Fprintln(os.Stderr, "Using placeholder values — edit the output before applying.")
		return kubeadm.Config_builder{
			Server:                   to.Ptr(placeholder),
			CertificateAuthorityData: []byte(placeholder),
			Token:                    to.Ptr(placeholder),
		}.Build()
	}
	cfg, err := kubeadmutil.FromAKS(ctx, credentials)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not retrieve kubeadm config from AKS cluster: %v\n", err)
		fmt.Fprintln(os.Stderr, "Using placeholder values — edit the output before applying.")
		return kubeadm.Config_builder{
			Server:                   to.Ptr(placeholder),
			CertificateAuthorityData: []byte(placeholder),
			Token:                    to.Ptr(placeholder),
		}.Build()
	}
	return cfg
}

func azureConfigTenantID() string {
	azureConfigDir := os.Getenv("AZURE_CONFIG_DIR")
	if azureConfigDir == "" {
		azureConfigDir = filepath.Join(os.Getenv("HOME"), ".azure")
	}

	b, err := os.ReadFile(filepath.Join(azureConfigDir, "azureProfile.json"))
	if err != nil {
		return ""
	}

	var profile struct {
		Subscriptions []struct {
			IsDefault bool   `json:"isDefault"`
			TenantID  string `json:"tenantId"`
		} `json:"subscriptions"`
	}
	if err := json.Unmarshal(b, &profile); err != nil {
		return ""
	}

	for _, sub := range profile.Subscriptions {
		if sub.IsDefault && sub.TenantID != "" {
			return sub.TenantID
		}
	}

	if len(profile.Subscriptions) == 1 {
		return profile.Subscriptions[0].TenantID
	}

	return ""
}
