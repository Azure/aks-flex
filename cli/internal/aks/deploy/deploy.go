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

	deploycilium      bool
	deployWireguard   bool
	deployGPUOperator bool

	//go:embed assets/aks.json
	aksJSON []byte
)

func init() {
	Command.Flags().BoolVar(&deploycilium, "cilium", true, "deploy Cilium CNI") // default to true to allow minimal networking to work
	Command.Flags().BoolVar(&deployWireguard, "wireguard", false, "deploy WireGuard gateway node pool and DaemonSet")
	Command.Flags().BoolVar(&deployGPUOperator, "gpu-operator", false, "install NVIDIA GPU Operator via Helm")
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

	if deploycilium {
		if err := deployCilium(ctx, credentials, cfg); err != nil {
			return err
		}
	}

	if deployWireguard {
		if err := deployWireGuard(ctx, credentials, cfg); err != nil {
			return err
		}
	}

	if deployGPUOperator {
		if err := installGPUOperator(ctx); err != nil {
			return err
		}
	}

	return nil
}
