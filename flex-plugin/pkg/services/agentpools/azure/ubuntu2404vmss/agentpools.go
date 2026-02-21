package ubuntu2404vmss

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v7"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
	agentpools "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/userdata/flex"
	"github.com/Azure/aks-flex/flex-plugin/pkg/topology"
	"github.com/Azure/aks-flex/flex-plugin/pkg/util/ssh"
)

var _ api.Object = (*AgentPool)(nil)

type agentpoolsServer struct {
	agentpools.UnimplementedAgentPoolsServer
	storage db.RODB

	credentials azcore.TokenCredential
}

func NewAgentPoolsServer(storage db.RODB) (agentpools.AgentPoolsServer, error) {
	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	return &agentpoolsServer{
		storage:     storage,
		credentials: credentials,
	}, nil
}

func (srv *agentpoolsServer) CreateOrUpdate(ctx context.Context, req *api.CreateOrUpdateRequest) (*api.CreateOrUpdateResponse, error) {
	ap, err := helper.AnyTo[*AgentPool](req.GetItem())
	if err != nil {
		return nil, err
	}

	rid, err := arm.ParseResourceID(ap.GetSpec().GetResourceId())
	if err != nil {
		return nil, err
	}

	vmss, err := armcompute.NewVirtualMachineScaleSetsClient(rid.SubscriptionID, srv.credentials, nil)
	if err != nil {
		return nil, err
	}

	sshKey, err := ssh.PublicKey()
	if err != nil {
		return nil, err
	}

	kubeadmConfig := ap.GetSpec().GetKubeadm()
	kubeadmConfig.AddNodeLabels(map[string]string{
		topology.NodeLabelKeyCloud:  "azure",
		topology.NodeLabelKeyRegion: strings.ToLower(ap.GetSpec().GetLocation()),
		// TODO: zone (might need to infer from created instance)
		topology.NodeLabelKeyInstanceType: strings.ToLower(ap.GetSpec().GetSku()),
	})

	// userData, err := ubuntu.UserData(kubeadmConfig)
	// if err != nil {
	// 	return nil, err
	// }
	userData, err := flex.UserData("1.33.3", kubeadmConfig)
	if err != nil {
		return nil, err
	}

	userDataContent, err := userData.Gzip()
	if err != nil {
		return nil, err
	}

	// Build VMSS spec
	vmssSpec := armcompute.VirtualMachineScaleSet{
		Location: to.Ptr(ap.GetSpec().GetLocation()),
		Properties: &armcompute.VirtualMachineScaleSetProperties{
			UpgradePolicy: &armcompute.UpgradePolicy{
				Mode: to.Ptr(armcompute.UpgradeModeManual),
			},
			VirtualMachineProfile: &armcompute.VirtualMachineScaleSetVMProfile{
				NetworkProfile: &armcompute.VirtualMachineScaleSetNetworkProfile{
					NetworkInterfaceConfigurations: []*armcompute.VirtualMachineScaleSetNetworkConfiguration{
						{
							Name: to.Ptr("nic"),
							Properties: &armcompute.VirtualMachineScaleSetNetworkConfigurationProperties{
								Primary: to.Ptr(true),
								IPConfigurations: []*armcompute.VirtualMachineScaleSetIPConfiguration{
									{
										Name: to.Ptr("ipconfig"),
										Properties: &armcompute.VirtualMachineScaleSetIPConfigurationProperties{
											Subnet: &armcompute.APIEntityReference{
												ID: to.Ptr(ap.GetSpec().GetSubnetId()),
											},
										},
									},
								},
							},
						},
					},
				},
				OSProfile: &armcompute.VirtualMachineScaleSetOSProfile{
					AdminUsername:      to.Ptr("ubuntu"),
					ComputerNamePrefix: &rid.Name,
					LinuxConfiguration: &armcompute.LinuxConfiguration{
						DisablePasswordAuthentication: to.Ptr(true),
						SSH: &armcompute.SSHConfiguration{
							PublicKeys: []*armcompute.SSHPublicKey{
								{
									Path:    to.Ptr("/home/ubuntu/.ssh/authorized_keys"),
									KeyData: to.Ptr(string(sshKey)),
								},
							},
						},
					},
				},
				StorageProfile: &armcompute.VirtualMachineScaleSetStorageProfile{
					ImageReference: &armcompute.ImageReference{
						Offer:     to.Ptr("ubuntu-24_04-lts"),
						Publisher: to.Ptr("Canonical"),
						SKU:       to.Ptr("server"),
						Version:   to.Ptr("latest"),
					},
					OSDisk: &armcompute.VirtualMachineScaleSetOSDisk{
						CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
						Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
						ManagedDisk: &armcompute.VirtualMachineScaleSetManagedDiskParameters{
							StorageAccountType: to.Ptr(armcompute.StorageAccountTypesPremiumLRS),
						},
					},
				},
				UserData: to.Ptr(base64.StdEncoding.EncodeToString(userDataContent)),
			},
		},
		SKU: &armcompute.SKU{
			Capacity: to.Ptr(int64(ap.GetSpec().GetCapacity().GetCapacity())),
			Name:     to.Ptr(ap.GetSpec().GetSku()),
			Tier:     to.Ptr("Standard"),
		},
	}

	// Conditionally set zones if specified
	zones := ap.GetSpec().GetZones()
	if len(zones) > 0 {
		vmssZones := make([]*string, len(zones))
		for i, z := range zones {
			vmssZones[i] = to.Ptr(z)
		}
		vmssSpec.Zones = vmssZones
	}

	poller, err := vmss.BeginCreateOrUpdate(ctx, rid.ResourceGroupName, rid.Name, vmssSpec, nil)
	if err != nil {
		return nil, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	item, err := anypb.New(ap)
	if err != nil {
		return nil, err
	}

	return api.CreateOrUpdateResponse_builder{
		Item: item,
	}.Build(), nil
}

func (srv *agentpoolsServer) Delete(ctx context.Context, req *api.DeleteRequest) (*api.DeleteResponse, error) {
	obj, ok := srv.storage.Get(req.GetId())
	if !ok {
		return api.DeleteResponse_builder{}.Build(), nil
	}

	ap, err := helper.To[*AgentPool](obj)
	if err != nil {
		return nil, err
	}

	rid, err := arm.ParseResourceID(ap.GetSpec().GetResourceId())
	if err != nil {
		return nil, err
	}

	vmss, err := armcompute.NewVirtualMachineScaleSetsClient(rid.SubscriptionID, srv.credentials, nil)
	if err != nil {
		return nil, err
	}

	poller, err := vmss.BeginDelete(ctx, rid.ResourceGroupName, rid.Name, &armcompute.VirtualMachineScaleSetsClientBeginDeleteOptions{
		ForceDeletion: to.Ptr(true),
	})
	if err != nil {
		return nil, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return api.DeleteResponse_builder{}.Build(), nil
}
