package unboundedcni

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"text/template"

	"github.com/spf13/cobra"
)

//go:embed assets/site.yaml
var siteTemplate string

// Command is the "config unbounded-cni" parent command.
var Command = &cobra.Command{
	Use:   "unbounded-cni",
	Short: "Unbounded CNI configuration commands",
}

var (
	flagName     string
	flagNodeCIDR string
	flagPodCIDR  string
)

var siteCmd = &cobra.Command{
	Use:   "site",
	Short: "Render an unbounded CNI Site resource",
	Long: `Render an unbounded CNI Site resource manifest.

The node CIDR should match the VNet address space on the Azure side.
The pod CIDR specifies the pod IP range assigned to nodes in this site.

If flags are not provided, placeholder values are used that must be
replaced before applying.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return renderSite(cmd.OutOrStdout())
	},
}

func init() {
	siteCmd.Flags().StringVar(&flagName, "name", "site-remote", "site resource name (e.g. site-azure)")
	siteCmd.Flags().StringVar(&flagNodeCIDR, "node-cidr", "172.20.0.0/16", "node CIDR block (e.g. 172.16.0.0/16)")
	siteCmd.Flags().StringVar(&flagPodCIDR, "pod-cidr", "10.200.0.0/16", "pod CIDR block (e.g. 10.100.0.0/16)")
	Command.AddCommand(siteCmd)
}

type siteContext struct {
	Name     string
	NodeCIDR string
	PodCIDR  string
}

func renderSite(w io.Writer) error {
	sc := &siteContext{
		Name:     orPlaceholder(flagName),
		NodeCIDR: orPlaceholder(flagNodeCIDR),
		PodCIDR:  orPlaceholder(flagPodCIDR),
	}

	tmpl, err := template.New("site").Parse(siteTemplate)
	if err != nil {
		return fmt.Errorf("parsing site template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, sc); err != nil {
		return fmt.Errorf("rendering site template: %w", err)
	}

	_, err = io.Copy(w, &buf)
	return err
}

func orPlaceholder(val string) string {
	if val != "" {
		return val
	}
	return "<replace-with-actual-value>"
}
