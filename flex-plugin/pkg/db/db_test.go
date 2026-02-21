package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/db/internal/testobj"
)

func newObj(id string) api.Object {
	return testobj.FakeObject_builder{
		Metadata: api.Metadata_builder{
			Id: proto.String(id),
		}.Build(),
	}.Build()
}

func tempDB(t *testing.T) *StupidDB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db := NewStupidDB(path)
	t.Cleanup(db.Close)
	return db
}

func TestStupidDB_ListEmpty(t *testing.T) {
	db := tempDB(t)

	objs := db.List()
	require.Empty(t, objs)
}

func TestStupidDB_CreateAndGet(t *testing.T) {
	db := tempDB(t)

	db.CreateOrUpdate(newObj("obj-1"))

	got, ok := db.Get("obj-1")
	require.True(t, ok)
	require.Equal(t, "obj-1", got.GetMetadata().GetId())
}

func TestStupidDB_CreateAndList(t *testing.T) {
	db := tempDB(t)

	db.CreateOrUpdate(newObj("obj-1"))
	db.CreateOrUpdate(newObj("obj-2"))

	objs := db.List()
	require.Len(t, objs, 2)

	ids := []string{objs[0].GetMetadata().GetId(), objs[1].GetMetadata().GetId()}
	require.ElementsMatch(t, []string{"obj-1", "obj-2"}, ids)
}

func TestStupidDB_UpdateExisting(t *testing.T) {
	db := tempDB(t)

	db.CreateOrUpdate(newObj("obj-1"))
	db.CreateOrUpdate(newObj("obj-1"))

	objs := db.List()
	require.Len(t, objs, 1)
	require.Equal(t, "obj-1", objs[0].GetMetadata().GetId())
}

func TestStupidDB_GetNotFound(t *testing.T) {
	db := tempDB(t)

	_, ok := db.Get("nonexistent")
	require.False(t, ok)
}

func TestStupidDB_Delete(t *testing.T) {
	db := tempDB(t)

	db.CreateOrUpdate(newObj("obj-1"))
	db.CreateOrUpdate(newObj("obj-2"))

	db.Delete("obj-1")

	objs := db.List()
	require.Len(t, objs, 1)
	require.Equal(t, "obj-2", objs[0].GetMetadata().GetId())
}

func TestStupidDB_DeleteNonexistent(t *testing.T) {
	db := tempDB(t)

	db.CreateOrUpdate(newObj("obj-1"))

	// Should not panic or alter existing data.
	db.Delete("nonexistent")

	objs := db.List()
	require.Len(t, objs, 1)
}

func TestStupidDB_DeleteAll(t *testing.T) {
	db := tempDB(t)

	db.CreateOrUpdate(newObj("obj-1"))
	db.Delete("obj-1")

	objs := db.List()
	require.Empty(t, objs)
}

func TestStupidDB_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db1 := NewStupidDB(path)
	db1.CreateOrUpdate(newObj("obj-1"))
	db1.Close()

	db2 := NewStupidDB(path)
	defer db2.Close()

	got, ok := db2.Get("obj-1")
	require.True(t, ok)
	require.Equal(t, "obj-1", got.GetMetadata().GetId())
}

func TestNewStupidDBWithSecret(t *testing.T) {
	cli := kubefake.NewSimpleClientset()

	db := NewStupidDBWithSecret(cli, "default", "my-db")
	defer db.Close()

	// Empty list on fresh store.
	require.Empty(t, db.List())

	// CRUD round-trip.
	db.CreateOrUpdate(newObj("obj-1"))
	db.CreateOrUpdate(newObj("obj-2"))

	objs := db.List()
	require.Len(t, objs, 2)

	got, ok := db.Get("obj-1")
	require.True(t, ok)
	require.Equal(t, "obj-1", got.GetMetadata().GetId())

	db.Delete("obj-1")
	objs = db.List()
	require.Len(t, objs, 1)
	require.Equal(t, "obj-2", objs[0].GetMetadata().GetId())
}
