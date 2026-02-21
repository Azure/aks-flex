package deploy

import (
	"context"
	_ "embed"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/flex-plugin/pkg/util/az"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/config"
)

var (
	Command = &cobra.Command{
		Use: "deploy",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context())
		},
	}

	deployWireguard   bool
	enableGPUOperator bool

	//go:embed assets/aks.json
	aksJSON []byte
)

func init() {
	Command.Flags().BoolVar(&deployWireguard, "wireguard", deployWireguard, "deploy WireGuard gateway node pool and DaemonSet")
	Command.Flags().BoolVar(&enableGPUOperator, "enable-gpu-operator", enableGPUOperator, "install NVIDIA GPU Operator via Helm")
}

func run(ctx context.Context) error {
	cfg, err := config.New()
	if err != nil {
		return err
	}

	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return err
	}

	if err := az.Deploy(ctx, credentials, cfg, "aks", aksJSON, map[string]*armresources.DeploymentParameter{
		"clusterName": {
			Value: cfg.ClusterName,
		},
		"vmSize": {
			Value: cfg.AKSNodeVMSize,
		},
		"deployWireguard": {
			Value: deployWireguard,
		},
	}); err != nil {
		return err
	}

	if err := deployK8S(ctx, credentials, cfg); err != nil {
		return err
	}

	if deployWireguard {
		if err := deployWireGuard(ctx, credentials, cfg); err != nil {
			return err
		}
	}

	if enableGPUOperator {
		if err := installGPUOperator(ctx); err != nil {
			return err
		}
	}

	return nil
}
