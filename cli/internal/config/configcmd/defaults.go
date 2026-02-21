package configcmd

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"

	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api/features/kubeadm"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/config"
	kubeadmutil "github.com/Azure/aks-flex/flex-plugin/pkg/util/kubeadm"
)

const placeholder = "<replace-with-actual-value>"

// DefaultConfig attempts to load the shared [config.Config] from environment
// variables. If validation fails (e.g. Azure env vars not set) it returns nil
// and prints a warning to stderr.
func DefaultConfig() *config.Config {
	cfg, err := config.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config from environment: %v\n", err)
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
	cfg, err := kubeadmutil.FromAKS(ctx)
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
