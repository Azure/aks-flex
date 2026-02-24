package plugin

import (
	"context"

	"github.com/nebius/gosdk"
	"github.com/nebius/gosdk/auth"
	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/cli/internal/plugin/apply"
	"github.com/Azure/aks-flex/cli/internal/plugin/delete"
	"github.com/Azure/aks-flex/cli/internal/plugin/get"
	utilconfig "github.com/Azure/aks-flex/flex-plugin/pkg/util/config"
	utilnebius "github.com/Azure/aks-flex/flex-plugin/pkg/util/nebius"
)

var Command = &cobra.Command{
	Use: "plugin",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// TODO: define the sdk setup pattern

		cfg, err := utilconfig.New()
		if err != nil {
			return err
		}

		if err := setupNebiusSDK(cmd.Context(), cfg); err != nil {
			return err
		}

		return nil
	},
}

func setupNebiusSDK(ctx context.Context, cfg *utilconfig.Config) error {
	sdk, err := gosdk.New(ctx,
		gosdk.WithCredentials(
			gosdk.ServiceAccountReader(
				auth.NewServiceAccountCredentialsFileParser(nil, cfg.NebiusCredentialsFile),
			),
		),
	)
	if err != nil {
		return err
	}

	utilnebius.SetSDKDoNotUseInProd(sdk)

	return nil
}

func init() {
	Command.AddCommand(get.Command)
	Command.AddCommand(apply.Command)
	Command.AddCommand(delete.Command)
}
