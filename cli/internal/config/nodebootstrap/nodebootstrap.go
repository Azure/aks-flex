package nodebootstrap

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/userdata/flex"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/userdata/ubuntu"

	"github.com/Azure/aks-flex/cli/internal/config/configcmd"
)

var r = configcmd.NewRouter("node-bootstrap", "Generate a node bootstrap cloud-init config for a remote cloud")

func init() {
	r.Handle("generic", writeUbuntuUserData)
	r.Handle("aws", writeUbuntuUserData)
	r.Handle("azure", writeFlexUserData)
	r.Handle("nebius", writeUbuntuUserData)
}

var Command *cobra.Command = r.Command()

func writeFlexUserData(ctx context.Context, w io.Writer) error {
	ud, err := flex.UserData("1.33.3", configcmd.DefaultKubeadmConfig(ctx))
	if err != nil {
		return fmt.Errorf("generating flex userdata: %w", err)
	}
	data, err := ud.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling userdata: %w", err)
	}
	_, err = w.Write(data)
	return err
}

func writeUbuntuUserData(ctx context.Context, w io.Writer) error {
	ud, err := ubuntu.UserData(configcmd.DefaultKubeadmConfig(ctx))
	if err != nil {
		return fmt.Errorf("generating ubuntu userdata: %w", err)
	}
	data, err := ud.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling userdata: %w", err)
	}
	_, err = w.Write(data)
	return err
}
