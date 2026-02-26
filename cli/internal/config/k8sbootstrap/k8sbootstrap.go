// Package k8sbootstrap provides the Kubernetes bootstrap resources (RBAC,
// ConfigMaps, bootstrap token Secret) required for kubeadm node join.
//
// It is used by both the "config k8s-bootstrap" CLI subcommand (which dumps
// the resources to stdout) and the deploy flow (which applies them to the
// cluster).
package k8sbootstrap

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"os"
	"text/template"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
	"github.com/Azure/aks-flex/plugin/pkg/util/az"
	"github.com/Azure/aks-flex/plugin/pkg/util/config"
	"github.com/Azure/aks-flex/plugin/pkg/util/k8s"
)

//go:embed assets/config.yaml
var configYAML string

// Command is the "config k8s-bootstrap" cobra command.
var Command = &cobra.Command{
	Use:   "k8s-bootstrap",
	Short: "Generate Kubernetes bootstrap resources for kubeadm node join",
	Long: `Generate Kubernetes bootstrap resources for kubeadm node join.

Outputs RBAC rules, cluster-info / kubeadm-config / kubelet-config ConfigMaps,
and a bootstrap token Secret. Apply these to the AKS cluster before joining
remote nodes.

Values are populated from the live AKS cluster when reachable. Otherwise
placeholder values are used that must be replaced before applying.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return writeConfig(cmd.Context(), cmd.OutOrStdout())
	},
}

// Params holds the values needed to render the k8s bootstrap template.
type Params struct {
	CertificateAuthorityData string
	Server                   string
	KubernetesVersion        string
	ServiceSubnet            string
	TokenID                  string
	TokenSecret              string
}

// Render renders the RBAC + ConfigMap + bootstrap token Secret YAML template
// with the given parameters.
func Render(p Params) ([]byte, error) {
	t, err := template.New("config").Parse(configYAML)
	if err != nil {
		return nil, fmt.Errorf("parsing config template: %w", err)
	}

	buf := &bytes.Buffer{}
	if err := t.Execute(buf, map[string]any{
		"certificateAuthorityData": p.CertificateAuthorityData,
		"server":                   p.Server,
		"kubernetesVersion":        p.KubernetesVersion,
		"serviceSubnet":            p.ServiceSubnet,
		"tokenID":                  p.TokenID,
		"tokenSecret":              p.TokenSecret,
	}); err != nil {
		return nil, fmt.Errorf("rendering config template: %w", err)
	}
	return buf.Bytes(), nil
}

// TokenID returns a deterministic 6-char bootstrap token ID derived from the
// Azure subscription and resource group, matching the convention used by the
// deploy flow.
func TokenID(subscriptionID, resourceGroup string) string {
	h := sha256.New()
	fmt.Fprintf(h, "/subscriptions/%s/resourceGroups/%s", subscriptionID, resourceGroup)
	return hex.EncodeToString(h.Sum(nil))[:6]
}

// BootstrapTokenSecret builds a bootstrap token Secret. If an existing Secret
// is found in the cluster (via cli) its token-secret is reused; otherwise a
// new random 16-char secret is generated. Pass a nil cli to always generate a
// new secret.
func BootstrapTokenSecret(ctx context.Context, cfg *config.Config, cli client.Client) (*corev1.Secret, error) {
	tokenID := TokenID(cfg.SubscriptionID, cfg.ResourceGroupName)

	var tokenSecret string
	if cli != nil {
		existing := &corev1.Secret{}
		err := cli.Get(ctx, client.ObjectKey{
			Namespace: metav1.NamespaceSystem,
			Name:      bootstrapapi.BootstrapTokenSecretPrefix + tokenID,
		}, existing)
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		tokenSecret = string(existing.Data[bootstrapapi.BootstrapTokenSecretKey])
	}

	if len(tokenSecret) != 16 {
		var err error
		tokenSecret, err = RandomString(16)
		if err != nil {
			return nil, err
		}
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      bootstrapapi.BootstrapTokenSecretPrefix + tokenID,
			Labels: map[string]string{
				k8s.LabelBootstrapToken: "true",
			},
		},
		Data: map[string][]byte{
			bootstrapapi.BootstrapTokenExtraGroupsKey:      []byte("system:bootstrappers:kubeadm:default-node-token"),
			bootstrapapi.BootstrapTokenIDKey:               []byte(tokenID),
			bootstrapapi.BootstrapTokenSecretKey:           []byte(tokenSecret),
			bootstrapapi.BootstrapTokenUsageAuthentication: []byte("true"),
			bootstrapapi.BootstrapTokenUsageSigningKey:     []byte("true"),
		},
		Type: corev1.SecretTypeBootstrapToken,
	}, nil
}

// RandomString generates a random alphanumeric string of the given length.
func RandomString(n int) (string, error) {
	const letters = "0123456789abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 0, n)
	for range n {
		r, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		b = append(b, letters[r.Int64()])
	}
	return string(b), nil
}

// ---- CLI command implementation ----

func writeConfig(ctx context.Context, w io.Writer) error {
	params := paramsFromContext(ctx)

	data, err := Render(params)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func paramsFromContext(ctx context.Context) Params {
	cfg := configcmd.DefaultConfig()
	if cfg == nil {
		return placeholderParams()
	}

	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		warn("could not obtain Azure credentials: %v", err)
		return placeholderParams()
	}

	// Kubeconfig for CA data + server URL.
	kubeconfig, err := k8s.Kubeconfig(ctx, credentials, cfg)
	if err != nil {
		warn("could not retrieve kubeconfig from AKS: %v", err)
		return placeholderParams()
	}
	cluster := kubeconfig.Clusters[kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster]

	// Kubernetes version + service subnet from the live cluster.
	params, err := paramsFromCluster(ctx, credentials, cfg, cluster)
	if err != nil {
		warn("%v", err)
		return placeholderParams()
	}

	// Bootstrap token.
	params.TokenID = TokenID(cfg.SubscriptionID, cfg.ResourceGroupName)
	params.TokenSecret, err = RandomString(16)
	if err != nil {
		warn("could not generate token secret: %v", err)
		return placeholderParams()
	}

	return *params
}

func paramsFromCluster(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config, cluster *clientcmdapi.Cluster) (*Params, error) {
	loader, err := k8s.Loader(ctx, credentials, cfg)
	if err != nil {
		return nil, fmt.Errorf("could not create kubeconfig loader: %w", err)
	}

	restconfig, err := loader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("could not build rest config: %w", err)
	}

	dis, err := discovery.NewDiscoveryClientForConfig(restconfig)
	if err != nil {
		return nil, fmt.Errorf("could not create discovery client: %w", err)
	}

	version, err := dis.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("could not discover Kubernetes version: %w", err)
	}

	mc, err := az.ManagedCluster(ctx, credentials, cfg)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve managed cluster: %w", err)
	}

	serviceSubnet := configcmd.OrPlaceholder("")
	if mc.Properties != nil && mc.Properties.NetworkProfile != nil && mc.Properties.NetworkProfile.ServiceCidr != nil {
		serviceSubnet = *mc.Properties.NetworkProfile.ServiceCidr
	}

	return &Params{
		CertificateAuthorityData: base64.StdEncoding.EncodeToString(cluster.CertificateAuthorityData),
		Server:                   cluster.Server,
		KubernetesVersion:        version.String(),
		ServiceSubnet:            serviceSubnet,
	}, nil
}

func placeholderParams() Params {
	p := configcmd.OrPlaceholder("")
	return Params{
		CertificateAuthorityData: p,
		Server:                   p,
		KubernetesVersion:        p,
		ServiceSubnet:            p,
		TokenID:                  p,
		TokenSecret:              p,
	}
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
	fmt.Fprintln(os.Stderr, "Using placeholder values — edit the output before applying.")
}
