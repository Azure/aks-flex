package ubuntu2404instance

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Azure/aks-flex/plugin/api"
	"github.com/Azure/aks-flex/plugin/pkg/db"
	"github.com/Azure/aks-flex/plugin/pkg/helper"
	agentpools "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api"
)

var _ api.Object = (*Instance)(nil)

type instancesServer struct {
	agentpools.UnimplementedInstancesServer
	storage db.RODB
}

func NewInstancesServer(storage db.RODB) (agentpools.InstancesServer, error) {
	return &instancesServer{
		storage: storage,
	}, nil
}

func (srv *instancesServer) List(ctx context.Context, req *api.ListRequest) (*api.ListResponse, error) {
	ap, ok := srv.storage.Get(req.GetId())
	if !ok {
		return nil, status.Error(codes.NotFound, "")
	}

	item, err := anypb.New(Instance_builder{
		Metadata: api.Metadata_builder{
			Id: to.Ptr(ap.GetMetadata().GetId() + "/0"),
		}.Build(),
	}.Build())
	if err != nil {
		return nil, err
	}

	return api.ListResponse_builder{
		Items: []*anypb.Any{item},
	}.Build(), nil
}

func (srv *instancesServer) Get(ctx context.Context, req *api.GetRequest) (*api.GetResponse, error) {
	ids := strings.Split(req.GetId(), "/")

	_, ok := srv.storage.Get(ids[0])
	if !ok || ids[1] != "0" {
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

	awscfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(ap.GetSpec().GetRegion()))
	if err != nil {
		return nil, err
	}

	client := ec2.NewFromConfig(awscfg)

	_, err = client.RebootInstances(ctx, &ec2.RebootInstancesInput{
		InstanceIds: []string{ap.GetStatus().GetInstance()},
	})
	if err != nil {
		return nil, err
	}

	return agentpools.InstanceRestartResponse_builder{}.Build(), nil
}
