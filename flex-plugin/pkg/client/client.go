package client

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services"
	agentpoolsapi "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
	networksapi "github.com/Azure/aks-flex/flex-plugin/pkg/services/networks/api"
	peeringsapi "github.com/Azure/aks-flex/flex-plugin/pkg/services/peerings/api"
)

type Client interface {
	CreateOrUpdate(context.Context, *api.CreateOrUpdateRequest, ...grpc.CallOption) (*api.CreateOrUpdateResponse, error)
	List(context.Context, *api.ListRequest, ...grpc.CallOption) (*api.ListResponse, error)
	Get(context.Context, *api.GetRequest, ...grpc.CallOption) (*api.GetResponse, error)
	Delete(context.Context, *api.DeleteRequest, ...grpc.CallOption) (*api.DeleteResponse, error)
}

func Get(name string) (client Client, _ error) {
	conn, err := services.NewConnection()
	if err != nil {
		return nil, err
	}

	switch name {
	case "agentpools":
		return agentpoolsapi.NewAgentPoolsClient(conn), nil
	case "networks":
		return networksapi.NewNetworksClient(conn), nil
	case "peerings":
		return peeringsapi.NewPeeringsClient(conn), nil
	}

	return nil, fmt.Errorf("unknown type %q", name)
}
