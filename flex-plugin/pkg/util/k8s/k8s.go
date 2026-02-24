package k8s

import (
	"context"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v8"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-flex/flex-plugin/pkg/util/config"
)

// LabelBootstrapToken is the label key used to identify bootstrap token secrets.
const LabelBootstrapToken = "flex.aks.azure.com/bootstrap-token"

func Client(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config) (client.Client, error) {
	kubeconfig, err := Kubeconfig(ctx, credentials, cfg)
	if err != nil {
		return nil, err
	}

	restconfig, err := clientcmd.NewDefaultClientConfig(*kubeconfig, nil).ClientConfig()
	if err != nil {
		return nil, err
	}

	return client.New(restconfig, client.Options{})
}

func getKubeconfig(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config) ([]byte, error) {
	managedClusters, err := armcontainerservice.NewManagedClustersClient(cfg.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, err
	}

	creds, err := managedClusters.ListClusterAdminCredentials(ctx, cfg.ResourceGroupName, cfg.ClusterName, nil)
	if err != nil {
		return nil, err
	}

	return creds.Kubeconfigs[0].Value, nil
}

func Kubeconfig(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config) (*api.Config, error) {
	creds, err := getKubeconfig(ctx, credentials, cfg)
	if err != nil {
		return nil, err
	}

	return clientcmd.Load(creds)
}

func Loader(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config) (clientcmd.ClientConfig, error) {
	kubeconfig, err := Kubeconfig(ctx, credentials, cfg)
	if err != nil {
		return nil, err
	}

	return clientcmd.NewDefaultClientConfig(*kubeconfig, nil), nil
}

// SaveKubeconfigTo saves the cluster admin kubeconfig to the specified file path.
func SaveKubeconfigTo(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config, path string) error {
	kubeconfig, err := Kubeconfig(ctx, credentials, cfg)
	if err != nil {
		return err
	}

	content, err := clientcmd.Write(*kubeconfig)
	if err != nil {
		return err
	}

	return os.WriteFile(path, content, 0600)
}
