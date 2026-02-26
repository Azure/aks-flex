package k8s

import (
	"context"
	"errors"
	"io"
	"iter"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyObject applies a single Kubernetes object to the cluster using
// server-side apply with ownership enforced.
func ApplyObject(
	ctx context.Context,
	cli client.Client,
	obj client.Object,
	fieldOwner string,
) error {
	return cli.Patch(
		ctx, obj,
		client.Apply, // nolint:staticcheck // TODO: migrate to ApplyConfigurationFromUnstructured
		client.FieldOwner(fieldOwner),
		client.ForceOwnership,
	)
}

// ApplyUnstructuredObjects applies the given unstructured Kubernetes objects
// to the cluster using server-side apply with ownership enforced.
// It fails on the first error encountered.
func ApplyUnstructuredObjects(
	ctx context.Context,
	cli client.Client,
	objects iter.Seq2[client.Object, error],
	fieldOwner string, // used for server-side apply to track ownership of fields
) error {
	for obj, err := range objects {
		if err != nil {
			return err
		}

		if err := ApplyObject(ctx, cli, obj, fieldOwner); err != nil {
			return err
		}
	}
	return nil
}

func decodeObjectsFromYAML(r io.Reader) iter.Seq2[client.Object, error] {
	return func(yield func(client.Object, error) bool) {
		decoder := yaml.NewYAMLToJSONDecoder(r)
		for {
			obj := &unstructured.Unstructured{}

			err := decoder.Decode(obj)
			if errors.Is(err, io.EOF) {
				// end of stream, stop iteration
				return
			}
			if !yield(obj, err) {
				return
			}
		}
	}
}

// ApplyYAMLSpec applies a YAML spec to the cluster as unstructured objects.
// This is similar to `kubectl apply -f <spec-file>` but with server-side apply.
func ApplyYAMLSpec(
	ctx context.Context,
	cli client.Client,
	spec io.Reader,
	fieldOwner string,
) error {
	objectsStream := decodeObjectsFromYAML(spec)
	return ApplyUnstructuredObjects(ctx, cli, objectsStream, fieldOwner)
}
