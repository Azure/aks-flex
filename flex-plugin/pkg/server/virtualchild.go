package server

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/db"
)

// VirtualChild implements aggregated CRUD operations for second-level (child)
// objects whose state is not persisted in a database.  CreateOrUpdate and
// Delete are not implemented.  For List/Get, it determines the object type of
// the parent, then calls the back-end server registered for the child.
type VirtualChild[T server] struct {
	DB      db.DB
	Servers map[string]T
}

var _ server = (*VirtualChild[server])(nil)

func NewVirtualChild[T server](db_ db.DB) *VirtualChild[T] {
	return &VirtualChild[T]{
		DB:      db_,
		Servers: map[string]T{},
	}
}

func (srv *VirtualChild[T]) CreateOrUpdate(ctx context.Context, req *api.CreateOrUpdateRequest) (*api.CreateOrUpdateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method CreateOrUpdate not implemented")
}

func (srv *VirtualChild[T]) List(ctx context.Context, req *api.ListRequest) (*api.ListResponse, error) {
	obj, ok := srv.DB.Get(req.GetId())
	if !ok {
		return nil, status.Error(codes.NotFound, "")
	}

	srv2, ok := srv.Servers[TypeURL(obj)]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "")
	}

	return srv2.List(ctx, req)
}

func (srv *VirtualChild[T]) Get(ctx context.Context, req *api.GetRequest) (*api.GetResponse, error) {
	ids := strings.Split(req.GetId(), "/")

	obj, ok := srv.DB.Get(ids[0])
	if !ok {
		return nil, status.Error(codes.NotFound, "")
	}

	srv2, ok := srv.Servers[TypeURL(obj)]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "")
	}

	return srv2.Get(ctx, req)
}

func (srv *VirtualChild[T]) Delete(ctx context.Context, req *api.DeleteRequest) (*api.DeleteResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method Delete not implemented")
}
