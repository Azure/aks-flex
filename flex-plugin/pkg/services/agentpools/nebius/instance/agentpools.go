package instance

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/nebius/gosdk"
	nebiuscommon "github.com/nebius/gosdk/proto/nebius/common/v1"
	nebiuscompute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	nebiuscomputeservice "github.com/nebius/gosdk/services/nebius/compute/v1"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
	agentpools "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/userdata/flex"
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

func (srv *agentPoolsServer) CreateOrUpdate(
	ctx context.Context,
	req *api.CreateOrUpdateRequest,
) (*api.CreateOrUpdateResponse, error) {
	ap, err := helper.AnyTo[*AgentPool](req.GetItem())
	if err != nil {
		return nil, err
	}
	apSpec := ap.GetSpec()

	// TODO: validate / default spec

	agentPoolResources := resolveNebiusAgentPool(utilnebius.MustGetSDK(ctx), ap)

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
	// TODO: get gpu info from spec (might need to infer from SKU)
	hasGPU := strings.Contains(apSpec.GetImageFamily(), "cuda")
	ud, err := flex.UserData(hasGPU, "1.33.3", kubeadmConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate userdata: %w", err)
	}
	// ud, err := ubuntu.UserData(kubeadmConfig)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to generate userdata: %w", err)
	// }

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

	bootDisk, err := agentPoolResources.DiskCRUD.CreateOrUpdate(ctx, utilnebius.DriftTODO, agentPoolResources.DesiredBootDisk())
	if err != nil {
		return nil, err
	}

	desiredInstance := agentPoolResources.DesiredInstance(bootDisk, string(userdataContent))
	instance, err := agentPoolResources.InstanceCRUD.CreateOrUpdate(ctx, utilnebius.DriftTODO, desiredInstance)
	if err != nil {
		return nil, err
	}

	status := ap.GetStatus()
	if status == nil {
		status = &AgentPoolStatus{}
	}
	status.SetInstanceId(instance.GetMetadata().GetId())
	status.SetOsDiskId(bootDisk.GetMetadata().GetId())
	status.SetCreatedAt(instance.GetMetadata().GetCreatedAt())
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

	ap, err := helper.To[*AgentPool](obj)
	if err != nil {
		return nil, err
	}

	agentPoolResources := resolveNebiusAgentPool(utilnebius.MustGetSDK(ctx), ap)
	osDisk := agentPoolResources.DesiredBootDisk()
	const emptyUserData = "" // deletion doesn't require user data
	instance := agentPoolResources.DesiredInstance(osDisk, emptyUserData)

	if err := agentPoolResources.InstanceCRUD.Delete(ctx, instance); err != nil {
		return nil, err
	}
	if err := agentPoolResources.DiskCRUD.Delete(ctx, osDisk); err != nil {
		return nil, err
	}

	return api.DeleteResponse_builder{}.Build(), nil
}

var (
	instanceCRUD = utilnebius.ResourceCRUDFactory[nebiuscomputeservice.InstanceService, *nebiuscompute.Instance]()
	diskCRUD     = utilnebius.ResourceCRUDFactory[nebiuscomputeservice.DiskService, *nebiuscompute.Disk]()
)

type nebiusAgentPoolResources struct {
	InstanceCRUD *utilnebius.ResourceCRUD[*nebiuscompute.Instance, *nebiuscompute.InstanceSpec]
	DiskCRUD     *utilnebius.ResourceCRUD[*nebiuscompute.Disk, *nebiuscompute.DiskSpec]

	AgentPool *AgentPool
}

func resolveNebiusAgentPool(sdk *gosdk.SDK, ap *AgentPool) *nebiusAgentPoolResources {
	return &nebiusAgentPoolResources{
		InstanceCRUD: instanceCRUD(sdk.Services().Compute().V1().Instance()),
		DiskCRUD:     diskCRUD(sdk.Services().Compute().V1().Disk()),
		AgentPool:    ap,
	}
}

func (res *nebiusAgentPoolResources) DesiredBootDisk() *nebiuscompute.Disk {
	return &nebiuscompute.Disk{
		Metadata: &nebiuscommon.ResourceMetadata{
			ParentId: res.AgentPool.GetSpec().GetProjectId(),
			Name:     fmt.Sprintf("%s-boot", res.AgentPool.GetMetadata().GetId()),
		},
		Spec: &nebiuscompute.DiskSpec{
			Size: &nebiuscompute.DiskSpec_SizeGibibytes{
				SizeGibibytes: res.AgentPool.GetSpec().GetOsDiskSizeGibibytes(),
			},
			Type: nebiuscompute.DiskSpec_NETWORK_SSD,
			Source: &nebiuscompute.DiskSpec_SourceImageFamily{
				SourceImageFamily: &nebiuscompute.SourceImageFamily{
					ImageFamily: res.AgentPool.GetSpec().GetImageFamily(),
				},
			},
		},
	}
}

func (res *nebiusAgentPoolResources) DesiredInstance(
	osDisk *nebiuscompute.Disk,
	userdata string,
) *nebiuscompute.Instance {
	nic := &nebiuscompute.NetworkInterfaceSpec{
		SubnetId:  res.AgentPool.GetSpec().GetSubnetId(),
		Name:      "eth0",
		IpAddress: &nebiuscompute.IPAddress{
			// Auto-allocate private IP
		},
	}
	// TODO: allow assigning public IP

	return &nebiuscompute.Instance{
		Metadata: &nebiuscommon.ResourceMetadata{
			ParentId: res.AgentPool.GetSpec().GetProjectId(),
			Name:     res.AgentPool.GetMetadata().GetId(),
		},
		Spec: &nebiuscompute.InstanceSpec{
			Resources: &nebiuscompute.ResourcesSpec{
				Platform: res.AgentPool.GetSpec().GetPlatform(),
				Size: &nebiuscompute.ResourcesSpec_Preset{
					Preset: res.AgentPool.GetSpec().GetPreset(),
				},
			},
			BootDisk: &nebiuscompute.AttachedDiskSpec{
				AttachMode: nebiuscompute.AttachedDiskSpec_READ_WRITE,
				Type: &nebiuscompute.AttachedDiskSpec_ExistingDisk{
					ExistingDisk: &nebiuscompute.ExistingDisk{
						Id: osDisk.GetMetadata().GetId(),
					},
				},
			},
			NetworkInterfaces: []*nebiuscompute.NetworkInterfaceSpec{
				nic,
			},
			CloudInitUserData: userdata,
		},
	}
}
