package db

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// maxChunkSize is the maximum size of data stored in a single Secret.
	// Kubernetes Secrets have a 1MiB etcd limit; we leave headroom for
	// metadata overhead.
	maxChunkSize = 1 << 20 // 1 MiB

	// secretDataKey is the key used inside each Secret's Data map.
	secretDataKey = "data"
)

// secretStore implements Store backed by Kubernetes Secrets.
// Data is gzipped and split into 1 MiB chunks stored as individual Secrets
// named <namePrefix>-0, <namePrefix>-1, etc.
type secretStore struct {
	mu         sync.Mutex
	cli        kubernetes.Interface
	namespace  string
	namePrefix string
}

var _ Store = (*secretStore)(nil)

// NewSecretStore returns a Store that persists data in Kubernetes Secrets.
// Data is gzip-compressed and chunked into secrets named <namePrefix>-0,
// <namePrefix>-1, etc. in the given namespace.
func NewSecretStore(cli kubernetes.Interface, namespace, namePrefix string) Store {
	return &secretStore{
		cli:        cli,
		namespace:  namespace,
		namePrefix: namePrefix,
	}
}

func (s *secretStore) Read() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.TODO()

	chunks, err := s.listChunks(ctx)
	if err != nil {
		return nil, err
	}

	if len(chunks) == 0 {
		return nil, nil
	}

	var compressed []byte
	for _, secret := range chunks {
		compressed = append(compressed, secret.Data[secretDataKey]...)
	}

	return decompress(compressed)
}

func (s *secretStore) Write(b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.TODO()

	compressed, err := compress(b)
	if err != nil {
		return fmt.Errorf("compressing data: %w", err)
	}

	chunks := chunk(compressed, maxChunkSize)

	secrets := s.cli.CoreV1().Secrets(s.namespace)

	// Write each chunk as a Secret.
	for i, data := range chunks {
		name := s.chunkName(i)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: s.namespace,
				Name:      name,
				Labels:    s.chunkLabels(),
			},
			Data: map[string][]byte{
				secretDataKey: data,
			},
		}

		existing, err := secrets.Get(ctx, name, metav1.GetOptions{})

		switch {
		case apierrors.IsNotFound(err):
			if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("creating secret %s: %w", name, err)
			}
		case err != nil:
			return fmt.Errorf("getting secret %s: %w", name, err)
		default:
			existing.Data = secret.Data
			existing.Labels = secret.Labels
			if _, err := secrets.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("updating secret %s: %w", name, err)
			}
		}
	}

	// Delete any leftover chunks from a previous write that had more chunks.
	if err := s.deleteChunksFrom(ctx, len(chunks)); err != nil {
		return err
	}

	return nil
}

func (s *secretStore) Close() error {
	return nil
}

// chunkName returns the Secret name for the given chunk index.
func (s *secretStore) chunkName(index int) string {
	return fmt.Sprintf("%s-%d", s.namePrefix, index)
}

// chunkLabels returns the labels applied to every chunk Secret so they can be
// listed together.
func (s *secretStore) chunkLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "stretch-db",
		"stretch-db/store":             s.namePrefix,
	}
}

// labelSelector returns a label selector string matching all chunks for this store.
func (s *secretStore) labelSelector() string {
	labels := s.chunkLabels()
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// listChunks returns all chunk Secrets for this store, sorted by name.
func (s *secretStore) listChunks(ctx context.Context) ([]corev1.Secret, error) {
	list, err := s.cli.CoreV1().Secrets(s.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: s.labelSelector(),
	})
	if err != nil {
		return nil, fmt.Errorf("listing chunk secrets: %w", err)
	}

	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Name < list.Items[j].Name
	})

	return list.Items, nil
}

// deleteChunksFrom deletes chunk Secrets starting from the given index onward.
func (s *secretStore) deleteChunksFrom(ctx context.Context, fromIndex int) error {
	secrets := s.cli.CoreV1().Secrets(s.namespace)

	for i := fromIndex; ; i++ {
		name := s.chunkName(i)
		err := secrets.Delete(ctx, name, metav1.DeleteOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("deleting stale secret %s: %w", name, err)
		}
	}
}

// chunk splits data into pieces of at most size bytes.
func chunk(data []byte, size int) [][]byte {
	if len(data) == 0 {
		return [][]byte{{}}
	}

	var chunks [][]byte
	for len(data) > 0 {
		end := min(len(data), size)
		chunks = append(chunks, data[:end])
		data = data[end:]
	}
	return chunks
}

func compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)

	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating gzip reader: %w", err)
	}
	defer r.Close()

	return io.ReadAll(r)
}
