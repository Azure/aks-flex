package server

import (
	"context"

	"google.golang.org/protobuf/proto"

	"github.com/Azure/aks-flex/plugin/api"
)

type server interface {
	CreateOrUpdate(context.Context, *api.CreateOrUpdateRequest) (*api.CreateOrUpdateResponse, error)
	List(context.Context, *api.ListRequest) (*api.ListResponse, error)
	Get(context.Context, *api.GetRequest) (*api.GetResponse, error)
	Delete(context.Context, *api.DeleteRequest) (*api.DeleteResponse, error)
}

func MustRegister[SRV any](m map[string]SRV, newServer func() (SRV, error), msg proto.Message) {
	srv, err := newServer()
	if err != nil {
		panic(err)
	}

	m[TypeURL(msg)] = srv
}

func TypeURL(msg proto.Message) string {
	return "type.googleapis.com/" + string(msg.ProtoReflect().Descriptor().FullName())
}
