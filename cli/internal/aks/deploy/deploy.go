package deploy

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/Azure/aks-flex/plugin/pkg/util/az"
	"github.com/Azure/aks-flex/plugin/pkg/util/config"
	"github.com/Azure/aks-flex/plugin/pkg/util/k8s"
)

var (
	Command = &cobra.Command{
		Use: "deploy",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context())
		},
	}

	deploycilium          bool
	deployWireguard       bool
	deployGPUOperator     bool
	deployGPUDevicePlugin bool
	skipARM               bool
	kubeconfigToSave      string

	//go:embed assets/aks.json
	aksJSON []byte
)

func init() {
	Command.Flags().BoolVar(&deploycilium, "cilium", false, "deploy Cilium CNI") // default to true to allow minimal networking to work
	Command.Flags().BoolVar(&deployWireguard, "wireguard", false, "deploy WireGuard gateway node pool and DaemonSet")
	Command.Flags().BoolVar(&deployGPUOperator, "gpu-operator", false, "install NVIDIA GPU Operator via Helm")
	Command.Flags().BoolVar(&deployGPUDevicePlugin, "gpu-device-plugin", false, "install NVIDIA GPU Device Plugin via Helm")
	Command.Flags().BoolVar(&skipARM, "skip-arm", false, "skip the ARM template deployment step")
	Command.Flags().MarkHidden("skip-arm")
	Command.Flags().StringVar(&kubeconfigToSave, "kubeconfig", "", "file path to write the cluster kubeconfig (default: ~/.kube/config, merged if already exists)")
}

func preflightChecks() error {
	if deploycilium {
		if err := preflightCiliumDeploy(); err != nil {
			return err
		}
	}
	if deployGPUOperator {
		if err := preflightGPUOperator(); err != nil {
			return err
		}
	}
	if deployGPUDevicePlugin {
		if err := preflightGPUDevicePlugin(); err != nil {
			return err
		}
	}

	return nil
}

func run(ctx context.Context) error {
	if err := preflightChecks(); err != nil {
		return err
	}

	cfg, err := config.New()
	if err != nil {
		return err
	}

	credentials, err := azidentity.NewAzureCLICredential(nil)
	if err != nil {
		return err
	}

	if !skipARM {
		log.Printf("Deploying AKS cluster %q in %q", cfg.ClusterName, cfg.ResourceGroupName)
		if err := az.Deploy(ctx, credentials, cfg, "aks", aksJSON, map[string]*armresources.DeploymentParameter{
			"clusterName": {
				Value: cfg.ClusterName,
			},
			"kubernetesVersion": {
				Value: cfg.ClusterVersion,
			},
			"systemPoolSize": {
				Value: cfg.SystemPoolSize,
			},
			"vmSize": {
				Value: cfg.SystemVMSize,
			},
			"wireguardVMSize": {
				Value: cfg.WireguardVMSize,
			},
			"deployWireguard": {
				Value: deployWireguard,
			},
		}); err != nil {
			return err
		}

		log.Printf("AKS cluster deployment complete")
	}

	kubeconfigPath, err := saveKubeconfig(ctx, credentials, cfg)
	if err != nil {
		return err
	}
	log.Printf("kubeconfig saved to %q", kubeconfigPath)

	if err := deployK8S(ctx, credentials, cfg); err != nil {
		return err
	}
	log.Printf("Kubernetes-side deployment complete")

	if deploycilium {
		if err := deployCilium(ctx, kubeconfigPath, cfg); err != nil {
			return err
		}
		log.Printf("Cilium deployment complete")
	}

	if deployWireguard {
		if err := deployWireGuard(ctx, credentials, cfg); err != nil {
			return err
		}
		log.Printf("WireGuard deployment complete")
	}

	if deployGPUOperator {
		if err := installGPUOperator(ctx); err != nil {
			return err
		}
		log.Printf("GPU Operator deployment complete")
	}

	if deployGPUDevicePlugin {
		if err := installGPUDevicePlugin(ctx); err != nil {
			return err
		}
		log.Printf("GPU Device Plugin deployment complete")
	}

	return nil
}

func saveKubeconfig(ctx context.Context, credentials azcore.TokenCredential, cfg *config.Config) (string, error) {
	outputPath := kubeconfigToSave
	if outputPath == "" {
		outputPath = clientcmd.RecommendedHomeFile
	}

	if err := k8s.MergeKubeconfigInto(ctx, credentials, cfg, outputPath); err != nil {
		return "", fmt.Errorf("failed to save kubeconfig to %s: %w", outputPath, err)
	}

	return outputPath, nil
}
