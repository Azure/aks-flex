package agentpools

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"google.golang.org/protobuf/proto"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api/features/capacity"
	azureap "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/azure/ubuntu2404vmss"
)

func newAzureAgentPool(ctx context.Context) proto.Message {
	cfg := configcmd.DefaultConfig()

	id := "azure-default"
	location := configcmd.OrPlaceholder("")
	sku := "Standard_D4s_v3"
	resourceID := ""
	var zones []string

	if cfg != nil {
		location = cfg.Location
		sku = cfg.StretchNodeVMSize
		zones = cfg.StretchNodeZones
		resourceID = fmt.Sprintf(
			"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachineScaleSets/%s",
			cfg.SubscriptionID, cfg.ResourceGroupName, id,
		)
	}

	return azureap.AgentPool_builder{
		Metadata: api.NewMetadata[*azureap.AgentPool](id),
		Spec: azureap.AgentPoolSpec_builder{
			ResourceId: to.Ptr(configcmd.OrPlaceholder(resourceID)),
			Location:   to.Ptr(configcmd.OrPlaceholder(location)),
			SubnetId:   to.Ptr(configcmd.OrPlaceholder("")),
			Sku:        to.Ptr(sku),
			Zones:      zones,
			Capacity: capacity.Config_builder{
				Capacity: to.Ptr(uint32(1)),
			}.Build(),
			Kubeadm: configcmd.DefaultKubeadmConfig(ctx),
		}.Build(),
	}.Build()
}
