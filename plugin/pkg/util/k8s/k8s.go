package k8s

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v8"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-flex/plugin/pkg/util/config"
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

// MergeKubeconfigInto merges the cluster admin kubeconfig into the kubeconfig file at path,
// adding/overwriting its clusters, users, and contexts, and setting the current context.
// If the file does not exist it is created.
func MergeKubeconfigInto(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config, path string) error {
	newKubeconfig, err := Kubeconfig(ctx, credentials, cfg)
	if err != nil {
		return err
	}

	existing, err := clientcmd.LoadFromFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		existing = api.NewConfig()
	}

	for k, v := range newKubeconfig.Clusters {
		existing.Clusters[k] = v
	}
	for k, v := range newKubeconfig.AuthInfos {
		existing.AuthInfos[k] = v
	}
	for k, v := range newKubeconfig.Contexts {
		existing.Contexts[k] = v
	}
	if newKubeconfig.CurrentContext != "" {
		existing.CurrentContext = newKubeconfig.CurrentContext
	}

	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}

	content, err := clientcmd.Write(*existing)
	if err != nil {
		return err
	}

	return os.WriteFile(path, content, 0600)
}

// APIServerFromKubeconfigFile returns the API server hostname and port from
// the kubeconfig file at path. If contextName is non-empty it is used to
// select the context; otherwise the file's current-context is used.
func APIServerFromKubeconfigFile(path, contextName string) (host, port string, err error) {
	kcfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return "", "", fmt.Errorf("loading kubeconfig for API server: %w", err)
	}

	ctxName := contextName
	if ctxName == "" {
		ctxName = kcfg.CurrentContext
	}
	if ctxName == "" {
		return "", "", errors.New("kubeconfig missing current context")
	}

	ctxCfg, ok := kcfg.Contexts[ctxName]
	if !ok || ctxCfg == nil {
		return "", "", fmt.Errorf("kubeconfig missing context %q", ctxName)
	}

	clusterCfg, ok := kcfg.Clusters[ctxCfg.Cluster]
	if !ok || clusterCfg == nil {
		return "", "", fmt.Errorf("kubeconfig missing cluster %q", ctxCfg.Cluster)
	}

	u, err := url.Parse(clusterCfg.Server)
	if err != nil {
		return "", "", fmt.Errorf("parsing API server URL %q: %w", clusterCfg.Server, err)
	}

	hostname := u.Hostname()
	p := u.Port()
	if hostname == "" {
		return "", "", fmt.Errorf("API server URL missing hostname: %q", clusterCfg.Server)
	}
	if p == "" {
		switch u.Scheme {
		case "https":
			p = "443"
		case "http":
			p = "80"
		default:
			return "", "", fmt.Errorf("API server URL missing port and unsupported scheme %q", u.Scheme)
		}
	}

	return hostname, p, nil
}
