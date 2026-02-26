package kubeadm

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/clientcmd/api/latest"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta4"

	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/kubeadm"
)

func Kubeconfig(cfg *kubeadm.Config) ([]byte, error) {
	const (
		cluster  = "cluster"
		context  = "context"
		authInfo = "user"
	)

	return runtime.Encode(latest.Codec, &api.Config{
		Clusters: map[string]*api.Cluster{
			cluster: {
				CertificateAuthorityData: cfg.GetCertificateAuthorityData(),
				Server:                   cfg.GetServer(),
			},
		},
		Contexts: map[string]*api.Context{
			context: {
				Cluster:  cluster,
				AuthInfo: authInfo,
			},
		},
		CurrentContext: context,
		AuthInfos: map[string]*api.AuthInfo{
			authInfo: {
				Token: cfg.GetToken(),
			},
		},
	})
}

func JoinConfig(cfg *kubeadm.Config, kubeConfigPath string) ([]byte, error) {
	scheme := runtime.NewScheme()

	scheme.AddKnownTypes(upstreamv1beta4.GroupVersion,
		&upstreamv1beta4.JoinConfiguration{},
	)

	codec := serializer.NewCodecFactory(scheme).CodecForVersions(
		json.NewYAMLSerializer(json.DefaultMetaFactory, scheme, scheme),
		nil,
		schema.GroupVersions{upstreamv1beta4.GroupVersion},
		nil,
	)

	// Build kubelet extra args
	var kubeletArgs []upstreamv1beta4.Arg

	// Add static node labels
	if l := cfg.GetNodeLabels(); len(l) > 0 {
		kubeletArgs = append(kubeletArgs, upstreamv1beta4.Arg{
			Name:  "node-labels",
			Value: nodeLabels(l),
		})
	}

	// Add --node-ip if configured (for WireGuard nodes)
	if cfg.HasNodeIp() {
		kubeletArgs = append(kubeletArgs, upstreamv1beta4.Arg{
			Name:  "node-ip",
			Value: cfg.GetNodeIp(),
		})
	}

	return runtime.Encode(codec, &upstreamv1beta4.JoinConfiguration{
		Discovery: upstreamv1beta4.Discovery{
			File: &upstreamv1beta4.FileDiscovery{
				KubeConfigPath: kubeConfigPath,
			},
		},
		NodeRegistration: upstreamv1beta4.NodeRegistrationOptions{
			Taints:           cfg.GetK8SRegisterTaints(),
			KubeletExtraArgs: kubeletArgs,
		},
	})
}

func nodeLabels(labels map[string]string) string {
	kv := make([]string, 0, len(labels))

	for k, v := range labels {
		kv = append(kv, k+"="+v)
	}

	return strings.Join(kv, ",")
}
