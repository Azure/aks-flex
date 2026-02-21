package networks

import (
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/server"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/networks/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/networks/aws/vpc"
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
		return vpc.NewNetworksServer(srv.DB)
	}, &vpc.Network{})

	return srv
}
