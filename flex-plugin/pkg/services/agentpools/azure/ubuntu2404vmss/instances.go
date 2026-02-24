package ubuntu2404vmss

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v7"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
	agentpools "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
)

var _ api.Object = (*Instance)(nil)

type instancesServer struct {
	agentpools.UnimplementedInstancesServer
	storage db.RODB

	credentials azcore.TokenCredential
}

func NewInstancesServer(storage db.RODB) (agentpools.InstancesServer, error) {
	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	return &instancesServer{
		storage:     storage,
		credentials: credentials,
	}, nil
}

func (srv *instancesServer) List(ctx context.Context, req *api.ListRequest) (*api.ListResponse, error) {
	obj, ok := srv.storage.Get(req.GetId())
	if !ok {
		return nil, status.Error(codes.NotFound, "")
	}

	ap, err := helper.To[*AgentPool](obj)
	if err != nil {
		return nil, err
	}

	rid, err := arm.ParseResourceID(ap.GetSpec().GetResourceId())
	if err != nil {
		return nil, err
	}

	vmss, err := armcompute.NewVirtualMachineScaleSetVMsClient(rid.SubscriptionID, srv.credentials, nil)
	if err != nil {
		return nil, err
	}

	var items []*anypb.Any
	pager := vmss.NewListPager(rid.ResourceGroupName, rid.Name, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, instance := range page.Value {
			obj := Instance_builder{
				Metadata: api.Metadata_builder{
					Id: to.Ptr(ap.GetMetadata().GetId() + "/" + *instance.InstanceID),
				}.Build(),
			}.Build()

			item, err := anypb.New(obj)
			if err != nil {
				return nil, err
			}

			items = append(items, item)
		}
	}

	return api.ListResponse_builder{
		Items: items,
	}.Build(), nil
}

func (srv *instancesServer) Get(ctx context.Context, req *api.GetRequest) (*api.GetResponse, error) {
	ids := strings.Split(req.GetId(), "/")

	_, ok := srv.storage.Get(ids[0])
	if !ok {
		return nil, status.Error(codes.NotFound, "")
	}

	item, err := anypb.New(Instance_builder{
		Metadata: api.Metadata_builder{
			Id: to.Ptr(req.GetId()),
		}.Build(),
	}.Build())
	if err != nil {
		return nil, err
	}

	return api.GetResponse_builder{
		Item: item,
	}.Build(), nil
}

func (srv *instancesServer) Restart(ctx context.Context, req *agentpools.InstanceRestartRequest) (*agentpools.InstanceRestartResponse, error) {
	ids := strings.Split(req.GetId(), "/")

	obj, ok := srv.storage.Get(ids[0])
	if !ok {
		return nil, status.Error(codes.NotFound, "")
	}

	ap, err := helper.To[*AgentPool](obj)
	if err != nil {
		return nil, err
	}

	rid, err := arm.ParseResourceID(ap.GetSpec().GetResourceId())
	if err != nil {
		return nil, err
	}

	vmssvms, err := armcompute.NewVirtualMachineScaleSetsClient(rid.SubscriptionID, srv.credentials, nil)
	if err != nil {
		return nil, err
	}

	poller, err := vmssvms.BeginRestart(ctx, rid.ResourceGroupName, rid.Name, &armcompute.VirtualMachineScaleSetsClientBeginRestartOptions{
		VMInstanceIDs: &armcompute.VirtualMachineScaleSetVMInstanceIDs{
			InstanceIDs: []*string{&ids[1]},
		},
	})
	if err != nil {
		return nil, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return agentpools.InstanceRestartResponse_builder{}.Build(), nil
}
