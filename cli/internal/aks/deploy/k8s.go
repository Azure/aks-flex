package deploy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"text/template"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/cluster-bootstrap/token/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-flex/flex-plugin/pkg/util/az"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/config"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/k8s"
)

//go:embed assets/config.yaml
var configYAML string

const fieldOwner = "stretch"

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

	t, err := template.New("config").Parse(configYAML)
	if err != nil {
		return err
	}

	buf := &bytes.Buffer{}
	if err := t.Execute(buf, map[string]any{
		"certificateAuthorityData": base64.StdEncoding.EncodeToString(kubeconfig.Clusters[kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster].CertificateAuthorityData),
		"server":                   kubeconfig.Clusters[kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster].Server,
		"kubernetesVersion":        kubernetesVersion.String(),
		"serviceSubnet":            *mc.Properties.NetworkProfile.ServiceCidr,
	}); err != nil {
		return err
	}

	if err := k8s.ApplyYAMLSpec(ctx, cli, buf, fieldOwner); err != nil {
		return err
	}

	bootstrapToken, err := bootstrapToken(ctx, cfg, cli)
	if err != nil {
		return err
	}
	if err := k8s.ApplyObject(ctx, cli, bootstrapToken, fieldOwner); err != nil {
		return err
	}

	return nil
}

func bootstrapToken(ctx context.Context, cfg *config.Config, cli client.Client) (*corev1.Secret, error) {
	h := sha256.New()
	if _, err := fmt.Fprintf(h, "/subscriptions/%s/resourceGroups/%s", cfg.SubscriptionID, cfg.ResourceGroupName); err != nil {
		return nil, err
	}
	tokenID := hex.EncodeToString(h.Sum(nil))[:6]

	secret := &corev1.Secret{}
	err := cli.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: api.BootstrapTokenSecretPrefix + tokenID}, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	tokenSecret := string(secret.Data[api.BootstrapTokenSecretKey])
	if len(tokenSecret) != 16 {
		var err error
		tokenSecret, err = randomString(16)
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
			Name:      api.BootstrapTokenSecretPrefix + tokenID,
			Labels: map[string]string{
				k8s.LabelBootstrapToken: "true",
			},
		},
		Data: map[string][]byte{
			api.BootstrapTokenExtraGroupsKey:      []byte("system:bootstrappers:kubeadm:default-node-token"),
			api.BootstrapTokenIDKey:               []byte(tokenID),
			api.BootstrapTokenSecretKey:           []byte(tokenSecret),
			api.BootstrapTokenUsageAuthentication: []byte("true"),
			api.BootstrapTokenUsageSigningKey:     []byte("true"),
		},
		Type: corev1.SecretTypeBootstrapToken,
	}, nil
}

func randomString(n int) (string, error) {
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
