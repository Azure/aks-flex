package db

import (
	"bytes"
	"context"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func fakeClient(t *testing.T, objs ...runtime.Object) kubernetes.Interface {
	t.Helper()
	return kubefake.NewSimpleClientset(objs...)
}

func newSecretStore(t *testing.T, objs ...runtime.Object) *secretStore {
	t.Helper()
	cli := fakeClient(t, objs...)
	return NewSecretStore(cli, "default", "test-db").(*secretStore)
}

func TestSecretStore_ReadEmpty(t *testing.T) {
	s := newSecretStore(t)

	b, err := s.Read()
	require.NoError(t, err)
	require.Nil(t, b)
}

func TestSecretStore_WriteAndRead(t *testing.T) {
	s := newSecretStore(t)

	data := []byte("hello world")
	require.NoError(t, s.Write(data))

	got, err := s.Read()
	require.NoError(t, err)
	require.Equal(t, data, got)
}

func TestSecretStore_WriteOverwrites(t *testing.T) {
	s := newSecretStore(t)

	require.NoError(t, s.Write([]byte("first")))
	require.NoError(t, s.Write([]byte("second")))

	got, err := s.Read()
	require.NoError(t, err)
	require.Equal(t, []byte("second"), got)
}

func TestSecretStore_WriteEmpty(t *testing.T) {
	s := newSecretStore(t)

	require.NoError(t, s.Write([]byte{}))

	got, err := s.Read()
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestSecretStore_Chunking(t *testing.T) {
	s := newSecretStore(t)

	// Write data that is larger than maxChunkSize when uncompressed.
	// We use incompressible-ish data to ensure it actually produces
	// multiple chunks after gzip.
	data := bytes.Repeat([]byte("abcdefghij"), maxChunkSize/5)
	// data is 2 MiB of repeating pattern; gzip compresses well so this
	// may fit in one chunk. Use random-like data instead.
	// Actually, let's just test the chunk helper and the round-trip with
	// a forced smaller chunk size.

	require.NoError(t, s.Write(data))

	got, err := s.Read()
	require.NoError(t, err)
	require.Equal(t, data, got)
}

func TestSecretStore_ChunkingProducesMultipleSecrets(t *testing.T) {
	s := newSecretStore(t)

	// Use random bytes which are incompressible.
	data := make([]byte, 3*maxChunkSize)
	_, err := rand.Read(data)
	require.NoError(t, err)

	require.NoError(t, s.Write(data))

	// Verify multiple secrets were created.
	chunks, err := s.listChunks(contextTODO())
	require.NoError(t, err)
	require.Greater(t, len(chunks), 1, "expected multiple chunk secrets")

	// Round-trip.
	got, err := s.Read()
	require.NoError(t, err)
	require.Equal(t, data, got)
}

func TestSecretStore_ShrinkingDeletesStaleChunks(t *testing.T) {
	s := newSecretStore(t)

	// Write large random data producing multiple chunks.
	large := make([]byte, 3*maxChunkSize)
	_, err := rand.Read(large)
	require.NoError(t, err)
	require.NoError(t, s.Write(large))

	largeChunks, err := s.listChunks(contextTODO())
	require.NoError(t, err)
	require.Greater(t, len(largeChunks), 1)

	// Write small data that fits in one chunk.
	require.NoError(t, s.Write([]byte("tiny")))

	smallChunks, err := s.listChunks(contextTODO())
	require.NoError(t, err)
	require.Equal(t, 1, len(smallChunks))

	got, err := s.Read()
	require.NoError(t, err)
	require.Equal(t, []byte("tiny"), got)
}

func TestSecretStore_Close(t *testing.T) {
	s := newSecretStore(t)
	require.NoError(t, s.Close())
}

func TestSecretStore_ChunkLabels(t *testing.T) {
	s := newSecretStore(t)

	require.NoError(t, s.Write([]byte("labeled")))

	chunks, err := s.listChunks(contextTODO())
	require.NoError(t, err)
	require.Len(t, chunks, 1)

	require.Equal(t, "stretch-db", chunks[0].Labels["app.kubernetes.io/managed-by"])
	require.Equal(t, "test-db", chunks[0].Labels["stretch-db/store"])
}

func TestSecretStore_IsolatedByPrefix(t *testing.T) {
	cli := fakeClient(t)

	s1 := NewSecretStore(cli, "default", "store-a").(*secretStore)
	s2 := NewSecretStore(cli, "default", "store-b").(*secretStore)

	require.NoError(t, s1.Write([]byte("data-a")))
	require.NoError(t, s2.Write([]byte("data-b")))

	got1, err := s1.Read()
	require.NoError(t, err)
	require.Equal(t, []byte("data-a"), got1)

	got2, err := s2.Read()
	require.NoError(t, err)
	require.Equal(t, []byte("data-b"), got2)
}

// --- helpers for compress/decompress/chunk unit tests ---

func TestCompress_Decompress_RoundTrip(t *testing.T) {
	data := []byte("hello, compressed world!")

	compressed, err := compress(data)
	require.NoError(t, err)
	require.NotEqual(t, data, compressed)

	decompressed, err := decompress(compressed)
	require.NoError(t, err)
	require.Equal(t, data, decompressed)
}

func TestChunk_SingleChunk(t *testing.T) {
	data := []byte("small")
	chunks := chunk(data, 100)
	require.Len(t, chunks, 1)
	require.Equal(t, data, chunks[0])
}

func TestChunk_MultipleChunks(t *testing.T) {
	data := []byte(strings.Repeat("x", 250))
	chunks := chunk(data, 100)
	require.Len(t, chunks, 3)
	require.Len(t, chunks[0], 100)
	require.Len(t, chunks[1], 100)
	require.Len(t, chunks[2], 50)
}

func TestChunk_ExactMultiple(t *testing.T) {
	data := []byte(strings.Repeat("x", 200))
	chunks := chunk(data, 100)
	require.Len(t, chunks, 2)
	require.Len(t, chunks[0], 100)
	require.Len(t, chunks[1], 100)
}

func TestChunk_Empty(t *testing.T) {
	chunks := chunk(nil, 100)
	require.Len(t, chunks, 1)
	require.Empty(t, chunks[0])
}

func contextTODO() context.Context {
	return context.TODO()
}
