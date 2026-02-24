package ipsecvpn

import (
	"context"
	_ "embed"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"google.golang.org/protobuf/types/known/anypb"

	api "github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
	peerings "github.com/Azure/aks-flex/flex-plugin/pkg/services/peerings/api"
	utilaws "github.com/Azure/aks-flex/flex-plugin/pkg/util/aws"
)

var _ api.Object = (*Peering)(nil)

//go:embed assets/aws.json
var awsJSON []byte

type peeringsServer struct {
	peerings.UnimplementedPeeringsServer
	storage db.RODB
}

func NewPeeringsServer(storage db.RODB) (peerings.PeeringsServer, error) {
	return &peeringsServer{
		storage: storage,
	}, nil
}

func (srv *peeringsServer) CreateOrUpdate(ctx context.Context, req *api.CreateOrUpdateRequest) (*api.CreateOrUpdateResponse, error) {
	p, err := helper.AnyTo[*Peering](req.GetItem())
	if err != nil {
		return nil, err
	}

	awscfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(p.GetSpec().GetRegion()))
	if err != nil {
		return nil, err
	}

	outputs, err := utilaws.Deploy(ctx, awscfg, "peering-"+p.GetMetadata().GetId(), awsJSON, []types.Parameter{
		{
			ParameterKey:   aws.String("PreSharedKey"),
			ParameterValue: aws.String(p.GetSpec().GetIpsec().GetPreSharedKey()),
		},
		{
			ParameterKey:   aws.String("RemoteIP"),
			ParameterValue: aws.String(p.GetSpec().GetIpsec().GetRemoteIp()),
		},
		{
			ParameterKey:   aws.String("RouteTable"),
			ParameterValue: aws.String(p.GetSpec().GetRouteTable()),
		},
		{
			ParameterKey:   aws.String("VPC"),
			ParameterValue: aws.String(p.GetSpec().GetVpc()),
		},
	})
	if err != nil {
		return nil, err
	}

	conn, err := utilaws.GetVpnConnection(ctx, awscfg, outputs["VPNConnection"])
	if err != nil {
		return nil, err
	}

	p.SetStatus(PeeringStatus_builder{
		VpnConnection: to.Ptr(outputs["VPNConnection"]),
		OutsideIpAddresses: []string{
			*conn.VgwTelemetry[0].OutsideIpAddress,
			*conn.VgwTelemetry[1].OutsideIpAddress,
		},
	}.Build())

	item, err := anypb.New(p)
	if err != nil {
		return nil, err
	}

	return api.CreateOrUpdateResponse_builder{
		Item: item,
	}.Build(), nil
}

func (srv *peeringsServer) Delete(ctx context.Context, req *api.DeleteRequest) (*api.DeleteResponse, error) {
	obj, ok := srv.storage.Get(req.GetId())
	if !ok {
		return api.DeleteResponse_builder{}.Build(), nil
	}

	n, err := helper.To[*Peering](obj)
	if err != nil {
		return nil, err
	}

	awscfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(n.GetSpec().GetRegion()))
	if err != nil {
		return nil, err
	}

	if err := utilaws.Delete(ctx, awscfg, "peering-"+n.GetMetadata().GetId()); err != nil {
		return nil, err
	}

	return api.DeleteResponse_builder{}.Build(), nil
}
