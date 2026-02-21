package get

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/client"
	"github.com/Azure/aks-flex/flex-plugin/pkg/helper"
	"github.com/Azure/aks-flex/flex-plugin/pkg/server"
)

var Command = &cobra.Command{
	Use:  "get",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return get(cmd.Context(), args)
	},
}

func get(ctx context.Context, args []string) error {
	client, err := client.Get(args[0])
	if err != nil {
		return err
	}

	if len(args) == 1 {
		items, err := helper.List[api.Object](client.List, ctx, "")
		if err != nil {
			return err
		}

		var objs []json.RawMessage
		for _, item := range items {
			item.GetMetadata().SetType(server.TypeURL(item))

			b, err := protojson.Marshal(item)
			if err != nil {
				return err
			}

			objs = append(objs, b)
		}

		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")

		if err := e.Encode(objs); err != nil {
			return err
		}

	} else {
		item, err := helper.Get[api.Object](client.Get, ctx, args[1])
		if err != nil {
			return err
		}

		item.GetMetadata().SetType(server.TypeURL(item))

		fmt.Println(protojson.Format(item))
	}

	return nil
}
