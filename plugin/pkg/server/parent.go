package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Azure/aks-flex/plugin/api"
	"github.com/Azure/aks-flex/plugin/pkg/db"
	"github.com/Azure/aks-flex/plugin/pkg/helper"
)

// Parent implements aggregated CRUD operations for top-level (parent) objects.
// For CreateOrUpdate/Delete, it determines the object type then calls the
// registered back-end server.  For List/Get, it serves from database.
type Parent[T server] struct {
	DB      db.DB
	Servers map[string]T
}

var _ server = (*Parent[server])(nil)

func NewParent[T server](db_ db.DB) *Parent[T] {
	return &Parent[T]{
		DB:      db_,
		Servers: map[string]T{},
	}
}

func (srv *Parent[T]) CreateOrUpdate(ctx context.Context, req *api.CreateOrUpdateRequest) (*api.CreateOrUpdateResponse, error) {
	srv2, ok := srv.Servers[req.GetItem().TypeUrl]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "")
	}

	resp, err := srv2.CreateOrUpdate(ctx, req)
	if err != nil {
		return resp, err
	}

	obj, err := helper.AnyTo[api.Object](resp.GetItem())
	if err != nil {
		return nil, err
	}

	srv.DB.CreateOrUpdate(obj)

	obj.Redact()

	item, err := anypb.New(obj)
	if err != nil {
		return nil, err
	}

	return api.CreateOrUpdateResponse_builder{
		Item: item,
	}.Build(), nil
}

func (srv *Parent[T]) List(ctx context.Context, req *api.ListRequest) (*api.ListResponse, error) {
	objs := srv.DB.List()

	items := make([]*anypb.Any, 0, len(objs))

	for _, obj := range objs {
		obj.Redact()

		item, err := anypb.New(obj)
		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return api.ListResponse_builder{
		Items: items,
	}.Build(), nil
}

func (srv *Parent[T]) Get(ctx context.Context, req *api.GetRequest) (*api.GetResponse, error) {
	obj, ok := srv.DB.Get(req.GetId())
	if !ok {
		return nil, status.Error(codes.NotFound, "")
	}

	obj.Redact()

	item, err := anypb.New(obj)
	if err != nil {
		return nil, err
	}

	return api.GetResponse_builder{
		Item: item,
	}.Build(), nil
}

func (srv *Parent[T]) Delete(ctx context.Context, req *api.DeleteRequest) (*api.DeleteResponse, error) {
	obj, ok := srv.DB.Get(req.GetId())
	if !ok {
		return api.DeleteResponse_builder{}.Build(), nil
	}

	srv2, ok := srv.Servers[TypeURL(obj)]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "")
	}

	resp, err := srv2.Delete(ctx, req)
	if err != nil {
		return resp, err
	}

	srv.DB.Delete(req.GetId())

	return resp, err
}
