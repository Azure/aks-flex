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

	deployGateway bool

	//go:embed assets/network.json
	networkJSON []byte
)

func init() {
	Command.Flags().BoolVar(&deployGateway, "gateway", deployGateway, "deploy gateway")
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

	return az.Deploy(ctx, credentials, cfg, "network", networkJSON, map[string]*armresources.DeploymentParameter{
		"deployGateway": {
			Value: deployGateway,
		},
	})
}
