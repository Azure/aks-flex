package agentpools

import (
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/server"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/aws/ubuntu2404instance"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/azure/ubuntu2404vmss"
	nebiusinstance "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/nebius/instance"
)

type agentPoolsServer struct {
	*server.Parent[api.AgentPoolsServer]
	api.UnsafeAgentPoolsServer
}

func NewAgentPoolsServer(db db.DB) api.AgentPoolsServer {
	srv := &agentPoolsServer{
		Parent: server.NewParent[api.AgentPoolsServer](db),
	}

	server.MustRegister(srv.Servers, func() (api.AgentPoolsServer, error) {
		return ubuntu2404instance.NewAgentPoolsServer(srv.DB)
	}, &ubuntu2404instance.AgentPool{})

	server.MustRegister(srv.Servers, func() (api.AgentPoolsServer, error) {
		return ubuntu2404vmss.NewAgentPoolsServer(srv.DB)
	}, &ubuntu2404vmss.AgentPool{})

	server.MustRegister(srv.Servers, func() (api.AgentPoolsServer, error) {
		return nebiusinstance.NewAgentPoolsServer(srv.DB)
	}, &nebiusinstance.AgentPool{})

	return srv
}
