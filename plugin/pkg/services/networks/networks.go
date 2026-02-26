package networks

import (
	"github.com/Azure/aks-flex/plugin/pkg/db"
	"github.com/Azure/aks-flex/plugin/pkg/server"
	"github.com/Azure/aks-flex/plugin/pkg/services/networks/api"
	awsvpc "github.com/Azure/aks-flex/plugin/pkg/services/networks/aws/vpc"
	nebiusvpc "github.com/Azure/aks-flex/plugin/pkg/services/networks/nebius/vpc"
)

type networksServer struct {
	*server.Parent[api.NetworksServer]
	api.UnsafeNetworksServer
}

func NewNetworksServer(db db.DB) api.NetworksServer {
	srv := &networksServer{
		Parent: server.NewParent[api.NetworksServer](db),
	}

	server.MustRegister(srv.Servers, func() (api.NetworksServer, error) {
		return awsvpc.NewNetworksServer(srv.DB)
	}, &awsvpc.Network{})

	server.MustRegister(srv.Servers, func() (api.NetworksServer, error) {
		return nebiusvpc.NewNetworksServer(srv.DB)
	}, &nebiusvpc.Network{})

	return srv
}
