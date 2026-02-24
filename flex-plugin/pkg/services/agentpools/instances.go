package agentpools

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
	"github.com/Azure/aks-flex/flex-plugin/pkg/server"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/aws/ubuntu2404instance"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/azure/ubuntu2404vmss"
)

type instancesServer struct {
	*server.VirtualChild[api.InstancesServer]
	api.UnsafeInstancesServer
}

func NewInstancesServer(db db.DB) api.InstancesServer {
	srv := &instancesServer{
		VirtualChild: server.NewVirtualChild[api.InstancesServer](db),
	}

	server.MustRegister(srv.Servers, func() (api.InstancesServer, error) {
		return ubuntu2404instance.NewInstancesServer(srv.DB)
	}, &ubuntu2404instance.AgentPool{})

	server.MustRegister(srv.Servers, func() (api.InstancesServer, error) {
		return ubuntu2404vmss.NewInstancesServer(srv.DB)
	}, &ubuntu2404vmss.AgentPool{})

	return srv
}

func (srv *instancesServer) Restart(ctx context.Context, req *api.InstanceRestartRequest) (*api.InstanceRestartResponse, error) {
	ids := strings.Split(req.GetId(), "/")

	obj, ok := srv.DB.Get(ids[0])
	if !ok {
		return nil, status.Error(codes.NotFound, "")
	}

	srv2, ok := srv.Servers[server.TypeURL(obj)]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "")
	}

	return srv2.Restart(ctx, req)
}
