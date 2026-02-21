package instance

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	compute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
	agentpools "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/userdata/ubuntu"
	"github.com/Azure/aks-flex/flex-plugin/pkg/topology"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/cloudinit"
	utilnebius "github.com/Azure/aks-flex/flex-plugin/pkg/util/nebius"
)

//go:embed assets/wg-spoke.sh
var wgSpokeScript string

var _ api.Object = (*AgentPool)(nil)

type agentPoolsServer struct {
	storage db.RODB

	agentpools.UnimplementedAgentPoolsServer
}

func NewAgentPoolsServer(storage db.RODB) (agentpools.AgentPoolsServer, error) {
	return &agentPoolsServer{
		storage: storage,
	}, nil
}

func instanceName(instance *AgentPool) string {
	return fmt.Sprintf("stretch-%s", instance.GetMetadata().GetId())
}

func (srv *agentPoolsServer) CreateOrUpdate(
	ctx context.Context,
	req *api.CreateOrUpdateRequest,
) (*api.CreateOrUpdateResponse, error) {
	ap, err := helper.AnyTo[*AgentPool](req.GetItem())
	if err != nil {
		return nil, err
	}
	apSpec := ap.GetSpec()

	kubeadmConfig := apSpec.GetKubeadm()
	kubeadmConfig.AddNodeLabels(map[string]string{
		topology.NodeLabelKeyCloud:  "nebius",
		topology.NodeLabelKeyRegion: strings.ToLower(apSpec.GetRegion()),
		// NOTE: this is to match nebius' naming pattern:
		// node.kubernetes.io/instance-type: cpu-d3
		topology.NodeLabelKeyInstanceType: apSpec.GetPlatform(),
	})

	wireguardIP := apSpec.GetWireguard().GetPeerIp()

	if wireguardIP != "" {
		// for wireguard enabled instance, the node IP needs to be set to the WireGuard peer IP,
		// so the network routing can work between nodes.
		kubeadmConfig.SetNodeIp(wireguardIP)
	}

	// TODO: get the k8s version from spec
	// ud, err := flex.UserData("1.33.3", kubeadmConfig)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to generate userdata: %w", err)
	// }
	ud, err := ubuntu.UserData(kubeadmConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate userdata: %w", err)
	}

	if wireguardIP != "" {
		// TODO: this part should move to flex node bootstrap setup task
		ud.Packages = append(ud.Packages, "wireguard", "wireguard-tools", "kubectl")
		ud.WriteFiles = append(ud.WriteFiles, &cloudinit.WriteFile{
			Path:        "/root/wg-spoke.sh",
			Content:     wgSpokeScript,
			Permissions: "0755",
		})
		ud.RunCmd = append(ud.RunCmd, strings.Join([]string{
			"export ANNOTATION_PREFIX='stretch.azure.com/wireguard-'",
			fmt.Sprintf("export WG_ADDRESS='%s/32'", wireguardIP),
			"export WG_DAEMONIZE='true'",
			"/root/wg-spoke.sh",
		}, "\n"))
	}

	userdataContent, err := ud.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cloud-init: %w", err)
	}

	instanceName := instanceName(ap)
	instanceConfig := utilnebius.InstanceConfig{
		ProjectID:     apSpec.GetProjectId(),
		Name:          instanceName, // FIXME: do we use the instance id as name?
		Platform:      apSpec.GetPlatform(),
		Preset:        apSpec.GetPreset(),
		SubnetID:      apSpec.GetSubnetId(),
		ImageFamily:   apSpec.GetImageFamily(),
		DiskSizeGB:    int64(apSpec.GetOsDiskSize()),
		CloudInitData: string(userdataContent),
	}

	sdk := utilnebius.MustGetSDK(ctx)

	nbInstance := utilnebius.NewInstance(sdk, instanceConfig)
	if err := nbInstance.Provision(ctx); err != nil {
		return nil, fmt.Errorf("failed to provision instance: %w", err)
	}

	// FIXME: remove nebius util layer
	instanceInfo, err := sdk.Services().Compute().V1().Instance().Get(
		ctx,
		&compute.GetInstanceRequest{
			Id: nbInstance.InstanceID(),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance info: %w", err)
	}

	status := AgentPoolStatus_builder{
		InstanceId: to.Ptr(nbInstance.InstanceID()),
		CreatedAt:  instanceInfo.Metadata.CreatedAt,
	}.Build()
	ap.SetStatus(status)

	item, err := anypb.New(ap)
	if err != nil {
		return nil, err
	}

	return api.CreateOrUpdateResponse_builder{Item: item}.Build(), nil
}

func (srv *agentPoolsServer) Delete(
	ctx context.Context,
	req *api.DeleteRequest,
) (*api.DeleteResponse, error) {
	obj, ok := srv.storage.Get(req.GetId())
	if !ok {
		return api.DeleteResponse_builder{}.Build(), nil
	}

	instance, err := helper.To[*AgentPool](obj)
	if err != nil {
		return nil, err
	}

	sdk := utilnebius.MustGetSDK(ctx)

	instanceName := instanceName(instance)
	nbInstance := utilnebius.NewInstance(sdk, utilnebius.InstanceConfig{
		ProjectID: instance.GetSpec().GetProjectId(),
		Name:      instanceName,
	})
	if err := nbInstance.Delete(ctx); err != nil {
		return nil, fmt.Errorf("failed to delete instance: %w", err)
	}

	return api.DeleteResponse_builder{}.Build(), nil
}
