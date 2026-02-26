package deploy

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/Azure/aks-flex/plugin/pkg/util/config"
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
	cmd := exec.CommandContext(
		ctx,
		"cilium", "install",
		"--set", "azure.resourceGroup="+cfg.ResourceGroupName,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(
		cmd.Env,
		"KUBECONFIG="+kubeconfigFile,
		"PATH="+os.Getenv("PATH"), // so cilium can find other tools
	)

	return cmd.Run()
}
