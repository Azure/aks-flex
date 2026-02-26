package peerings

import (
	"github.com/Azure/aks-flex/plugin/pkg/db"
	"github.com/Azure/aks-flex/plugin/pkg/server"
	"github.com/Azure/aks-flex/plugin/pkg/services/peerings/api"
	"github.com/Azure/aks-flex/plugin/pkg/services/peerings/aws/ipsecvpn"
)

type peeringsServer struct {
	*server.Parent[api.PeeringsServer]
	api.UnsafePeeringsServer
}

func NewPeeringsServer(db db.DB) api.PeeringsServer {
	srv := &peeringsServer{
		Parent: server.NewParent[api.PeeringsServer](db),
	}

	server.MustRegister(srv.Servers, func() (api.PeeringsServer, error) {
		return ipsecvpn.NewPeeringsServer(srv.DB)
	}, &ipsecvpn.Peering{})

	return srv
}
