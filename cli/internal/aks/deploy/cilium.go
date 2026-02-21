package deploy

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"

	"github.com/Azure/aks-flex/flex-plugin/pkg/util/config"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/k8s"
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
	cred azcore.TokenCredential,
	cfg *config.Config,
) error {
	kubeconfigFile, err := k8s.SaveKuebeconfig(ctx, cred, cfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(kubeconfigFile)
	}()

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
