package vpc

import (
	"context"
	_ "embed"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Azure/aks-flex/plugin/api"
	"github.com/Azure/aks-flex/plugin/pkg/db"
	"github.com/Azure/aks-flex/plugin/pkg/helper"
	networks "github.com/Azure/aks-flex/plugin/pkg/services/networks/api"
	utilaws "github.com/Azure/aks-flex/plugin/pkg/util/aws"
)

var _ api.Object = (*Network)(nil)

//go:embed assets/aws.json
var awsJSON []byte

type networksServer struct {
	networks.UnimplementedNetworksServer
	storage db.RODB
}

func NewNetworksServer(storage db.RODB) (networks.NetworksServer, error) {
	return &networksServer{
		storage: storage,
	}, nil
}

func (srv *networksServer) CreateOrUpdate(ctx context.Context, req *api.CreateOrUpdateRequest) (*api.CreateOrUpdateResponse, error) {
	n, err := helper.AnyTo[*Network](req.GetItem())
	if err != nil {
		return nil, err
	}

	awscfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(n.GetSpec().GetRegion()))
	if err != nil {
		return nil, err
	}

	outputs, err := utilaws.Deploy(ctx, awscfg, "network-"+n.GetMetadata().GetId(), awsJSON, []types.Parameter{
		{
			ParameterKey:   aws.String("CidrBlock"),
			ParameterValue: aws.String(n.GetSpec().GetVnet().GetCidrBlock()),
		},
	})
	if err != nil {
		return nil, err
	}

	n.SetStatus(NetworkStatus_builder{
		Vpc:           to.Ptr(outputs["VPC"]),
		Subnet:        to.Ptr(outputs["Subnet"]),
		RouteTable:    to.Ptr(outputs["RouteTable"]),
		SecurityGroup: to.Ptr(outputs["SecurityGroup"]),
	}.Build())

	item, err := anypb.New(n)
	if err != nil {
		return nil, err
	}

	return api.CreateOrUpdateResponse_builder{
		Item: item,
	}.Build(), nil
}

func (srv *networksServer) Delete(ctx context.Context, req *api.DeleteRequest) (*api.DeleteResponse, error) {
	obj, ok := srv.storage.Get(req.GetId())
	if !ok {
		return api.DeleteResponse_builder{}.Build(), nil
	}

	n, err := helper.To[*Network](obj)
	if err != nil {
		return nil, err
	}

	awscfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(n.GetSpec().GetRegion()))
	if err != nil {
		return nil, err
	}

	if err := utilaws.Delete(ctx, awscfg, "network-"+n.GetMetadata().GetId()); err != nil {
		return nil, err
	}

	return api.DeleteResponse_builder{}.Build(), nil
}
