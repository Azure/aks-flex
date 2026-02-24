package db

import (
	"slices"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/client-go/kubernetes"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
)

type RODB interface {
	List() []api.Object
	Get(string) (api.Object, bool)
}

type DB interface {
	RODB
	CreateOrUpdate(api.Object)
	Delete(string)
}

// StupidDB is a stupid persistence store for protobufs that support
// `GetMetadata().GetId()`.  It must not be used in production.  It is intended
// to be thrown away.
type StupidDB struct {
	mu    sync.Mutex
	store Store
}

var _ DB = (*StupidDB)(nil)

func NewStupidDB(filename string) *StupidDB {
	return &StupidDB{
		store: newLockedFileStore(filename),
	}
}

// NewStupidDBWithSecret creates a StupidDB backed by Kubernetes Secrets.
// Data is gzip-compressed and chunked into secrets named <namePrefix>-0,
// <namePrefix>-1, etc. in the given namespace.
func NewStupidDBWithSecret(cli kubernetes.Interface, namespace, namePrefix string) *StupidDB {
	return &StupidDB{
		store: NewSecretStore(cli, namespace, namePrefix),
	}
}

func (db *StupidDB) Close() {
	if err := db.store.Close(); err != nil {
		panic(err)
	}
}

func (db *StupidDB) CreateOrUpdate(obj api.Object) {
	db.mu.Lock()
	defer db.mu.Unlock()

	objs := slices.DeleteFunc(db.list(), func(o api.Object) bool { return o.GetMetadata().GetId() == obj.GetMetadata().GetId() })

	objs = append(objs, obj)

	db.write(objs)
}

func (db *StupidDB) List() []api.Object {
	db.mu.Lock()
	defer db.mu.Unlock()

	return db.list()
}

func (db *StupidDB) list() []api.Object {
	b, err := db.store.Read()
	if err != nil {
		panic(err)
	}

	list := &api.ListResponse{}
	if err := proto.Unmarshal(b, list); err != nil {
		panic(err)
	}

	objs := make([]api.Object, 0, len(list.GetItems()))

	for _, item := range list.GetItems() {
		obj, err := helper.AnyTo[api.Object](item)
		if err != nil {
			panic(err)
		}

		objs = append(objs, obj)
	}

	return objs
}

func (db *StupidDB) write(objs []api.Object) {
	items := make([]*anypb.Any, 0, len(objs))

	for _, obj := range objs {
		item, err := anypb.New(obj)
		if err != nil {
			panic(err)
		}

		items = append(items, item)
	}

	b, err := proto.Marshal(api.ListResponse_builder{
		Items: items,
	}.Build())
	if err != nil {
		panic(err)
	}

	if err := db.store.Write(b); err != nil {
		panic(err)
	}
}

func (db *StupidDB) Get(id string) (api.Object, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()

	for _, obj := range db.list() {
		if obj.GetMetadata().GetId() == id {
			return obj, true
		}
	}

	return nil, false
}

func (db *StupidDB) Delete(id string) {
	db.mu.Lock()
	defer db.mu.Unlock()

	objs := slices.DeleteFunc(db.list(), func(o api.Object) bool { return o.GetMetadata().GetId() == id })

	db.write(objs)
}
