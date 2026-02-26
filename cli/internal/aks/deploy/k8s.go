package deploy

import (
	"bytes"
	"context"
	"encoding/base64"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-flex/cli/internal/config/k8sbootstrap"
	"github.com/Azure/aks-flex/plugin/pkg/util/az"
	"github.com/Azure/aks-flex/plugin/pkg/util/config"
	"github.com/Azure/aks-flex/plugin/pkg/util/k8s"
)

const fieldOwner = "aks-flex-cli"

func deployK8S(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config) error {
	mc, err := az.ManagedCluster(ctx, credentials, cfg)
	if err != nil {
		return err
	}

	loader, err := k8s.Loader(ctx, credentials, cfg)
	if err != nil {
		return err
	}

	restconfig, err := loader.ClientConfig()
	if err != nil {
		return err
	}

	cli, err := client.New(restconfig, client.Options{})
	if err != nil {
		return err
	}

	dis, err := discovery.NewDiscoveryClientForConfig(restconfig)
	if err != nil {
		return err
	}

	kubernetesVersion, err := dis.ServerVersion()
	if err != nil {
		return err
	}

	kubeconfig, err := loader.RawConfig()
	if err != nil {
		return err
	}

	cluster := kubeconfig.Clusters[kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster]

	// Resolve the bootstrap token (reuses existing secret if present).
	bootstrapToken, err := k8sbootstrap.BootstrapTokenSecret(ctx, cfg, cli)
	if err != nil {
		return err
	}

	tokenID := string(bootstrapToken.Data["token-id"])
	tokenSecret := string(bootstrapToken.Data["token-secret"])

	data, err := k8sbootstrap.Render(k8sbootstrap.Params{
		CertificateAuthorityData: base64.StdEncoding.EncodeToString(cluster.CertificateAuthorityData),
		Server:                   cluster.Server,
		KubernetesVersion:        kubernetesVersion.String(),
		ServiceSubnet:            *mc.Properties.NetworkProfile.ServiceCidr,
		TokenID:                  tokenID,
		TokenSecret:              tokenSecret,
	})
	if err != nil {
		return err
	}

	return k8s.ApplyYAMLSpec(ctx, cli, bytes.NewReader(data), fieldOwner)
}
