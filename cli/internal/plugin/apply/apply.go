package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/client"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
)

var Command = &cobra.Command{
	Use:  "apply",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return apply(cmd.Context(), args)
	},
}

func apply(ctx context.Context, args []string) error {
	client, err := client.Get(args[0])
	if err != nil {
		return err
	}

	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	tok, err := json.NewDecoder(bytes.NewReader(b)).Token()
	if err != nil {
		return err
	}

	var bs []json.RawMessage
	if tok == json.Delim('[') {
		if err := json.Unmarshal(b, &bs); err != nil {
			return err
		}

	} else {
		bs = append(bs, b)
	}

	for _, b := range bs {
		obj, err := applyOne(ctx, client, b)
		if err != nil {
			return err
		}

		// TODO: improve UI feedback
		log.Printf("Applied %q (type: %s)", obj.GetMetadata().GetId(), obj.GetMetadata().GetType())
	}

	return nil
}

func applyOne(ctx context.Context, client client.Client, b []byte) (*api.Base, error) {
	obj := &api.Base{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(b, obj); err != nil {
		return nil, err
	}

	mt, err := protoregistry.GlobalTypes.FindMessageByURL(obj.GetMetadata().GetType())
	if err != nil {
		return nil, err
	}

	m := mt.New().Interface()
	if err := protojson.Unmarshal(b, m); err != nil {
		return nil, err
	}

	_, err = helper.CreateOrUpdate(client.CreateOrUpdate, ctx, m)
	if err != nil {
		return nil, err
	}

	return obj, nil
}
