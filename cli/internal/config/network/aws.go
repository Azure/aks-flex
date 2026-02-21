package network

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"google.golang.org/protobuf/proto"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/networks/api/features/vnet"
	awsvpc "github.com/Azure/aks-flex/flex-plugin/pkg/services/networks/aws/vpc"
)

func newAWSNetwork(_ context.Context) proto.Message {
	return awsvpc.Network_builder{
		Metadata: api.NewMetadata[*awsvpc.Network]("aws-default"),
		Spec: awsvpc.NetworkSpec_builder{
			Region: to.Ptr(configcmd.OrPlaceholder("")),
			Vnet: vnet.Config_builder{
				CidrBlock: to.Ptr(configcmd.OrPlaceholder("")),
			}.Build(),
		}.Build(),
	}.Build()
}
