package deploy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"

	"github.com/Azure/aks-flex/plugin/pkg/util/config"
	"k8s.io/client-go/tools/clientcmd"
)

var ciliumInstallInstruction = errors.New(
	"cilium cli not found, please follow instruction " +
		"https://docs.cilium.io/en/stable/gettingstarted/k8s-install-default/#install-the-cilium-cli",
)

func preflightCiliumDeploy() error {
	_, err := exec.LookPath("cilium")
	if err != nil {
		return ciliumInstallInstruction
	}

	return nil
}

func deployCilium(
	ctx context.Context,
	kubeconfigFile string,
	cfg *config.Config,
) error {
	k8sServiceHost, k8sServicePort, err := kubeconfigAPIServer(kubeconfigFile)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(
		ctx,
		"cilium", "install",
		"--kubeconfig", kubeconfigFile,
		"--context", cfg.ClusterName+"-admin",
		"--namespace", "kube-system",
		"--datapath-mode", "aks-byocni",
		"--helm-set", "aksbyocni.enabled=true",
		"--helm-set", "cluster.name="+cfg.ClusterName,
		"--helm-set", "operator.replicas=1",
		"--helm-set", "kubeProxyReplacement=true",
		"--helm-set", "k8sServiceHost="+k8sServiceHost,
		"--helm-set", "k8sServicePort="+k8sServicePort,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(
		cmd.Environ(),
		"KUBECONFIG="+kubeconfigFile,
		"PATH="+os.Getenv("PATH"),
	)
	log.Printf("Running: cilium install --kubeconfig %s --context %s --namespace kube-system --datapath-mode aks-byocni --helm-set aksbyocni.enabled=true --helm-set cluster.name=%s --helm-set operator.replicas=1 --helm-set kubeProxyReplacement=true --helm-set k8sServiceHost=%s --helm-set k8sServicePort=%s", kubeconfigFile, cfg.ClusterName+"-admin", cfg.ClusterName, k8sServiceHost, k8sServicePort)

	return cmd.Run()
}

func kubeconfigAPIServer(kubeconfigFile string) (string, string, error) {
	kcfg, err := clientcmd.LoadFromFile(kubeconfigFile)
	if err != nil {
		return "", "", fmt.Errorf("loading kubeconfig for cilium install: %w", err)
	}

	ctxName := kcfg.CurrentContext
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
	port := u.Port()
	if hostname == "" {
		return "", "", fmt.Errorf("API server URL missing hostname: %q", clusterCfg.Server)
	}
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return "", "", fmt.Errorf("API server URL missing port and unsupported scheme %q", u.Scheme)
		}
	}

	return hostname, port, nil
}
