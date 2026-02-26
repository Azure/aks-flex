package db

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func tempFileStore(t *testing.T) *lockedFileStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	fs := newLockedFileStore(path)
	t.Cleanup(func() { _ = fs.Close() })
	return fs
}

func TestLockedFileStore_ReadEmpty(t *testing.T) {
	fs := tempFileStore(t)

	b, err := fs.Read()
	require.NoError(t, err)
	require.Empty(t, b)
}

func TestLockedFileStore_WriteAndRead(t *testing.T) {
	fs := tempFileStore(t)

	data := []byte("hello world")
	require.NoError(t, fs.Write(data))

	got, err := fs.Read()
	require.NoError(t, err)
	require.Equal(t, data, got)
}

func TestLockedFileStore_WriteOverwrites(t *testing.T) {
	fs := tempFileStore(t)

	require.NoError(t, fs.Write([]byte("first write with a lot of data")))
	require.NoError(t, fs.Write([]byte("short")))

	got, err := fs.Read()
	require.NoError(t, err)
	require.Equal(t, []byte("short"), got)
}

func TestLockedFileStore_MultipleReads(t *testing.T) {
	fs := tempFileStore(t)

	require.NoError(t, fs.Write([]byte("data")))

	for i := 0; i < 3; i++ {
		got, err := fs.Read()
		require.NoError(t, err)
		require.Equal(t, []byte("data"), got)
	}
}

func TestLockedFileStore_Close(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	fs := newLockedFileStore(path)

	require.NoError(t, fs.Write([]byte("data")))
	require.NoError(t, fs.Close())

	// Reading after close panics because flock fails on a closed fd.
	require.Panics(t, func() { _, _ = fs.Read() })
}

func TestLockedFileStore_PersistenceAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	fs1 := newLockedFileStore(path)
	require.NoError(t, fs1.Write([]byte("persisted")))
	require.NoError(t, fs1.Close())

	fs2 := newLockedFileStore(path)
	defer func() { _ = fs2.Close() }()

	got, err := fs2.Read()
	require.NoError(t, err)
	require.Equal(t, []byte("persisted"), got)
}

func TestLockedFileStore_ConcurrentAccess(t *testing.T) {
	fs := tempFileStore(t)

	var wg sync.WaitGroup
	iterations := 50

	for i := 0; i < iterations; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			_ = fs.Write([]byte("writer-1"))
		}()

		go func() {
			defer wg.Done()
			_ = fs.Write([]byte("writer-2"))
		}()
	}

	wg.Wait()

	got, err := fs.Read()
	require.NoError(t, err)
	// The final content must be one of the two writers -- not corrupted.
	require.Contains(t, []string{"writer-1", "writer-2"}, string(got))
}

func TestLockedFileStore_CreatesFileIfNotExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.db")

	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))

	fs := newLockedFileStore(path)
	defer func() { _ = fs.Close() }()

	_, err = os.Stat(path)
	require.NoError(t, err)
}
