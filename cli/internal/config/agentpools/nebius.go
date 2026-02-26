package agentpools

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"google.golang.org/protobuf/proto"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
	"github.com/Azure/aks-flex/plugin/api"
	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/wireguard"
	nebiusap "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/nebius/instance"
)

func newNebiusAgentPool(ctx context.Context) proto.Message {
	cfg := configcmd.DefaultConfig()

	projectID := ""
	region := ""
	if cfg != nil {
		projectID = cfg.NebiusProjectID
		region = cfg.NebiusRegion
	}

	return nebiusap.AgentPool_builder{
		Metadata: api.NewMetadata[*nebiusap.AgentPool]("nebius-default"),
		Spec: nebiusap.AgentPoolSpec_builder{
			ProjectId:           to.Ptr(configcmd.OrPlaceholder(projectID)),
			Region:              to.Ptr(configcmd.OrPlaceholder(region)),
			SubnetId:            to.Ptr(configcmd.OrPlaceholder("")),
			Platform:            to.Ptr(configcmd.OrPlaceholder("")),
			Preset:              to.Ptr(configcmd.OrPlaceholder("")),
			ImageFamily:         to.Ptr(configcmd.OrPlaceholder("")),
			OsDiskSizeGibibytes: to.Ptr(int64(128)),
			Kubeadm:             configcmd.DefaultKubeadmConfig(ctx),
			Wireguard: wireguard.Config_builder{
				PeerIp: to.Ptr(configcmd.OrPlaceholder("")),
			}.Build(),
		}.Build(),
	}.Build()
}
