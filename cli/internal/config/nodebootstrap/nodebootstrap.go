package nodebootstrap

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/userdata/flex"
	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/userdata/ubuntu"
	"github.com/Azure/aks-flex/plugin/pkg/util/cloudinit"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
)

const (
	variantCloudInit = "cloud-init"
	variantScript    = "script"
)

var r = configcmd.NewRouter("node-bootstrap", "Generate a node bootstrap config for a remote cloud")
var Command *cobra.Command = r.Command()
var flagHasGPU bool
var flagVariant string
var flagArch string

func init() {
	r.Handle("ubuntu", writeUbuntuUserData)
	r.Handle("flex", writeFlexUserData)

	Command.Flags().BoolVar(&flagHasGPU, "gpu", false, "Indicates whether the node has GPU. This may affect the generated userdata.")
	Command.Flags().StringVar(&flagArch, "arch", "amd64",
		"CPU architecture for the flex node binary (e.g. amd64, arm64).")
	Command.Flags().StringVar(&flagVariant, "variant", variantCloudInit,
		fmt.Sprintf("Output variant: %q produces cloud-init YAML user data, %q produces an equivalent standalone bash script.", variantCloudInit, variantScript))
}

// marshalUserData marshals the cloud-init UserData according to the selected
// --variant and writes it to w.
func marshalUserData(ud *cloudinit.UserData, w io.Writer) error {
	var data []byte
	var err error

	switch flagVariant {
	case variantCloudInit:
		data, err = ud.Marshal()
	case variantScript:
		data, err = marshalScript(ud)
	default:
		return fmt.Errorf("unsupported variant %q, supported: %s, %s", flagVariant, variantCloudInit, variantScript)
	}
	if err != nil {
		return fmt.Errorf("marshaling userdata as %s: %w", flagVariant, err)
	}

	_, err = w.Write(data)
	return err
}

func writeFlexUserData(ctx context.Context, w io.Writer) error {
	ud, err := flex.UserData(
		flex.WithEnableNvidiaGPURuntime(flagHasGPU),
		flex.WithArch(flagArch),
		flex.WithKubeadmConfig(configcmd.DefaultKubeadmConfig(ctx)),
	)
	if err != nil {
		return fmt.Errorf("generating flex userdata: %w", err)
	}
	return marshalUserData(ud, w)
}

func writeUbuntuUserData(ctx context.Context, w io.Writer) error {
	ud, err := ubuntu.UserData(configcmd.DefaultKubeadmConfig(ctx))
	if err != nil {
		return fmt.Errorf("generating ubuntu userdata: %w", err)
	}
	return marshalUserData(ud, w)
}
