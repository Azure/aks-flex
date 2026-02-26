package delete

import (
	"context"
	"log"

	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/plugin/pkg/client"
	"github.com/Azure/aks-flex/plugin/pkg/helper"
)

var Command = &cobra.Command{
	Use:  "delete",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return delete(cmd.Context(), args)
	},
}

func delete(ctx context.Context, args []string) error {
	client, err := client.Get(args[0])
	if err != nil {
		return err
	}

	resourceName := args[1]
	log.Printf("Deleting %q...", resourceName)

	if err := helper.Delete(client.Delete, ctx, resourceName); err != nil {
		return err
	}
	log.Printf("Successfully deleted %q", resourceName)

	return nil
}
