package vpc

import (
	"context"
	"fmt"

	"github.com/nebius/gosdk"
	nebiuscommon "github.com/nebius/gosdk/proto/nebius/common/v1"
	nebiusvpc "github.com/nebius/gosdk/proto/nebius/vpc/v1"
	nebiusvpcservice "github.com/nebius/gosdk/services/nebius/vpc/v1"
	"google.golang.org/protobuf/types/known/anypb"

	api "github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
	networks "github.com/Azure/aks-flex/flex-plugin/pkg/services/networks/api"
	utilnebius "github.com/Azure/aks-flex/flex-plugin/pkg/util/nebius"
)

var _ api.Object = (*Network)(nil)

var (
	poolCRUD    = utilnebius.ResourceCRUDFactory[nebiusvpcservice.PoolService, *nebiusvpc.Pool]()
	networkCRUD = utilnebius.ResourceCRUDFactory[nebiusvpcservice.NetworkService, *nebiusvpc.Network]()
	subnetCRUD  = utilnebius.ResourceCRUDFactory[nebiusvpcservice.SubnetService, *nebiusvpc.Subnet]()
)

type networksServer struct {
	networks.UnimplementedNetworksServer
	storage db.RODB
}

func NewNetworksServer(storage db.RODB) (networks.NetworksServer, error) {
	return &networksServer{
		storage: storage,
	}, nil
}

func (srv *networksServer) CreateOrUpdate(
	ctx context.Context,
	req *api.CreateOrUpdateRequest,
) (*api.CreateOrUpdateResponse, error) {
	n, err := helper.AnyTo[*Network](req.GetItem())
	if err != nil {
		return nil, err
	}
	// TODO: validate / default spec

	networkResources := resolveNebiusNetwork(utilnebius.MustGetSDK(ctx), n)

	pool, err := networkResources.PoolCRUD.CreateOrUpdate(ctx, utilnebius.DriftTODO, networkResources.DesiredPool())
	if err != nil {
		return nil, err
	}
	network, err := networkResources.NetworkCRUD.CreateOrUpdate(ctx, utilnebius.DriftTODO, networkResources.DesiredNetwork(pool))
	if err != nil {
		return nil, err
	}
	subnet, err := networkResources.SubnetCRUD.CreateOrUpdate(ctx, utilnebius.DriftTODO, networkResources.DesiredSubnet(network))
	if err != nil {
		return nil, err
	}

	status := n.GetStatus()
	if status == nil {
		status = &NetworkStatus{}
	}
	status.SetPoolId(pool.GetMetadata().GetId())
	status.SetVnetId(network.GetMetadata().GetId())
	status.SetSubnetId(subnet.GetMetadata().GetId())
	n.SetStatus(status)

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
		// Object doesn't exist, consider it deleted
		return api.DeleteResponse_builder{}.Build(), nil
	}

	n, err := helper.To[*Network](obj)
	if err != nil {
		return nil, err
	}

	networkResources := resolveNebiusNetwork(utilnebius.MustGetSDK(ctx), n)
	pool := networkResources.DesiredPool()
	network := networkResources.DesiredNetwork(pool)
	subnet := networkResources.DesiredSubnet(network)

	if err := networkResources.SubnetCRUD.Delete(ctx, subnet); err != nil {
		return nil, err
	}
	if err := networkResources.NetworkCRUD.Delete(ctx, network); err != nil {
		return nil, err
	}
	if err := networkResources.PoolCRUD.Delete(ctx, pool); err != nil {
		return nil, err
	}

	return api.DeleteResponse_builder{}.Build(), nil
}

type nebiusNetworkResources struct {
	PoolCRUD    *utilnebius.ResourceCRUD[*nebiusvpc.Pool, *nebiusvpc.PoolSpec]
	NetworkCRUD *utilnebius.ResourceCRUD[*nebiusvpc.Network, *nebiusvpc.NetworkSpec]
	SubnetCRUD  *utilnebius.ResourceCRUD[*nebiusvpc.Subnet, *nebiusvpc.SubnetSpec]

	Network *Network
}

func resolveNebiusNetwork(sdk *gosdk.SDK, n *Network) *nebiusNetworkResources {
	return &nebiusNetworkResources{
		PoolCRUD:    poolCRUD(sdk.Services().VPC().V1().Pool()),
		NetworkCRUD: networkCRUD(sdk.Services().VPC().V1().Network()),
		SubnetCRUD:  subnetCRUD(sdk.Services().VPC().V1().Subnet()),
		Network:     n,
	}
}

func (res *nebiusNetworkResources) DesiredPool() *nebiusvpc.Pool {
	return &nebiusvpc.Pool{
		Metadata: &nebiuscommon.ResourceMetadata{
			Id:       res.Network.GetStatus().GetPoolId(),
			ParentId: res.Network.GetSpec().GetProjectId(),
			Name:     fmt.Sprintf("%s-pool", res.Network.GetMetadata().GetId()),
		},
		Spec: &nebiusvpc.PoolSpec{
			Version:    nebiusvpc.IpVersion_IPV4,
			Visibility: nebiusvpc.IpVisibility_PRIVATE,
			Cidrs: []*nebiusvpc.PoolCidr{
				{Cidr: res.Network.GetSpec().GetVnet().GetCidrBlock()},
			},
		},
	}
}

func (res *nebiusNetworkResources) DesiredNetwork(pool *nebiusvpc.Pool) *nebiusvpc.Network {
	rv := &nebiusvpc.Network{
		Metadata: &nebiuscommon.ResourceMetadata{
			Id:       res.Network.GetStatus().GetVnetId(),
			ParentId: res.Network.GetSpec().GetProjectId(),
			Name:     res.Network.GetMetadata().GetId(),
		},
		Spec: &nebiusvpc.NetworkSpec{
			Ipv4PrivatePools: &nebiusvpc.IPv4PrivateNetworkPools{
				Pools: []*nebiusvpc.NetworkPool{},
			},
		},
	}
	if poolID := pool.GetMetadata().GetId(); poolID != "" {
		rv.Spec.Ipv4PrivatePools.Pools = append(
			rv.Spec.Ipv4PrivatePools.Pools,
			&nebiusvpc.NetworkPool{
				Id: poolID,
			},
		)
	}
	return rv
}

func (res *nebiusNetworkResources) DesiredSubnet(network *nebiusvpc.Network) *nebiusvpc.Subnet {
	rv := &nebiusvpc.Subnet{
		Metadata: &nebiuscommon.ResourceMetadata{
			Id:       res.Network.GetStatus().GetSubnetId(),
			ParentId: res.Network.GetSpec().GetProjectId(),
			Name:     fmt.Sprintf("%s-subnet", res.Network.GetMetadata().GetId()),
		},
		Spec: &nebiusvpc.SubnetSpec{
			Ipv4PrivatePools: &nebiusvpc.IPv4PrivateSubnetPools{
				// Inherit from network pools (which uses our custom pool)
				// When UseNetworkPools is true, Pools must be empty
				UseNetworkPools: true,
			},
		},
	}
	if networkID := network.GetMetadata().GetId(); networkID != "" {
		rv.Spec.NetworkId = networkID
	}
	return rv
}
