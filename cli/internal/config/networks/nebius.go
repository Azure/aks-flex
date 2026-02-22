package networks

import (
	"context"

	"google.golang.org/protobuf/proto"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/networks/api/features/vnet"
	nebiusvpc "github.com/Azure/aks-flex/flex-plugin/pkg/services/networks/nebius/vpc"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
)

func newNebiusNetwork(ctx context.Context) proto.Message {
	cfg := configcmd.DefaultConfig()

	return nebiusvpc.Network_builder{
		Metadata: api.NewMetadata[*nebiusvpc.Network]("nebius-default"),
		Spec: nebiusvpc.NetworkSpec_builder{
			ProjectId: to.Ptr(configcmd.OrPlaceholder(cfg.NebiusProjectID)),
			Region:    to.Ptr(configcmd.OrPlaceholder(cfg.NebiusRegion)),
			Vnet: vnet.Config_builder{
				CidrBlock: to.Ptr("172.20.0.0/16"),
			}.Build(),
		}.Build(),
	}.Build()
}
