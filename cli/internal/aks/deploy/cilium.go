package deploy

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"

	"github.com/Azure/aks-flex/plugin/pkg/util/config"
	"github.com/Azure/aks-flex/plugin/pkg/util/k8s"
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
	clusterContext := cfg.ClusterName + "-admin"
	k8sServiceHost, k8sServicePort, err := k8s.APIServerFromKubeconfigFile(kubeconfigFile, clusterContext)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(
		ctx,
		"cilium", "install",
		"--kubeconfig", kubeconfigFile,
		"--context", clusterContext,
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
	log.Printf("Running: cilium install --kubeconfig %s --context %s --namespace kube-system --datapath-mode aks-byocni --helm-set aksbyocni.enabled=true --helm-set cluster.name=%s --helm-set operator.replicas=1 --helm-set kubeProxyReplacement=true --helm-set k8sServiceHost=%s --helm-set k8sServicePort=%s", kubeconfigFile, clusterContext, cfg.ClusterName, k8sServiceHost, k8sServicePort)

	return cmd.Run()
}
