package kubeadm

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/kubeadm"
	"github.com/Azure/aks-flex/plugin/pkg/topology"
	"github.com/Azure/aks-flex/plugin/pkg/util/az"
	"github.com/Azure/aks-flex/plugin/pkg/util/config"
	"github.com/Azure/aks-flex/plugin/pkg/util/k8s"
)

func FromAKS(ctx context.Context) (*kubeadm.Config, error) {
	cfg, err := config.New()
	if err != nil {
		return nil, err
	}

	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	kubeconfig, err := k8s.Kubeconfig(ctx, credentials, cfg)
	if err != nil {
		return nil, err
	}

	restconfig, err := clientcmd.NewDefaultClientConfig(*kubeconfig, nil).ClientConfig()
	if err != nil {
		return nil, err
	}

	cli, err := client.New(restconfig, client.Options{})
	if err != nil {
		return nil, err
	}

	secrets := &corev1.SecretList{}
	if err := cli.List(ctx, secrets, client.MatchingFields{"metadata.namespace": metav1.NamespaceSystem}, client.MatchingLabels{k8s.LabelBootstrapToken: "true"}); err != nil {
		return nil, err
	}

	if len(secrets.Items) != 1 {
		return nil, fmt.Errorf("did not find exactly one stretch bootstrap secret")
	}

	mc, err := az.ManagedCluster(ctx, credentials, cfg)
	if err != nil {
		return nil, err
	}
	if mc.Properties.NodeResourceGroup == nil {
		return nil, fmt.Errorf("nil node resource group returned from managed cluster")
	}

	return kubeadm.Config_builder{
		CertificateAuthorityData: kubeconfig.Clusters[kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster].CertificateAuthorityData,
		Server:                   to.Ptr(kubeconfig.Clusters[kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster].Server),
		Token:                    to.Ptr(string(secrets.Items[0].Data[bootstrapapi.BootstrapTokenIDKey]) + "." + string(secrets.Items[0].Data[bootstrapapi.BootstrapTokenSecretKey])),

		NodeLabels: map[string]string{
			topology.NodeLabelKeyCloudProviderManaged: "false",
			topology.NodeLabelKeyCloudProviderCluster: *mc.Properties.NodeResourceGroup,
			topology.NodeLabelKeyStretchManaged:       "true",
		},
	}.Build(), nil
}
