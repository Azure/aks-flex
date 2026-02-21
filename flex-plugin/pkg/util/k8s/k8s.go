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

// SaveKuebeconfig saves the kubeconfig to temporary file.
// The caller is responsible for deleting the temporary file after using it.
func SaveKuebeconfig(
	ctx context.Context,
	credentials azcore.TokenCredential,
	cfg *config.Config,
) (string, error) {
	kubeconfig, err := getKubeconfig(ctx, credentials, cfg)
	if err != nil {
		return "", err
	}

	tempfile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return "", err
	}
	defer tempfile.Close()

	if _, err := tempfile.Write(kubeconfig); err != nil {
		return "", err
	}

	return tempfile.Name(), nil
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
