// Package configcmd provides a router for building config subcommands that
// dispatch to cloud-specific handlers.
//
// Usage is similar to mounting routes on an HTTP mux:
//
//	r := configcmd.NewRouter("network", "Generate a default network config")
//	r.Handle("aws", func(ctx context.Context, w io.Writer) error { ... })
//	r.Handle("azure", func(ctx context.Context, w io.Writer) error { ... })
//	var Command = r.Command()
package configcmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Handler writes config output for a specific cloud to w.
type Handler func(ctx context.Context, w io.Writer) error

// Router maps cloud names to handlers and produces a cobra.Command.
type Router struct {
	use   string
	short string
	// Ordered so ValidArgs and help text are deterministic.
	clouds []string
	routes map[string]Handler
	cmd    *cobra.Command
}

// NewRouter creates a new Router for a config subcommand.
func NewRouter(use, short string) *Router {
	return &Router{
		use:    use + " <remote-cloud>",
		short:  short,
		routes: make(map[string]Handler),
	}
}

// Handle registers a handler for the given cloud name.
func (r *Router) Handle(cloud string, h Handler) {
	if _, exists := r.routes[cloud]; exists {
		panic(fmt.Sprintf("configcmd: duplicate handler for cloud %q", cloud))
	}
	r.clouds = append(r.clouds, cloud)
	r.routes[cloud] = h

	// Keep the command in sync with the registered clouds.
	if r.cmd != nil {
		r.cmd.ValidArgs = r.clouds
		r.cmd.Long = r.short + "\n\nSupported remote clouds: " + strings.Join(r.clouds, ", ")
	}
}

// Command builds the cobra.Command that dispatches to the registered handlers.
func (r *Router) Command() *cobra.Command {
	r.cmd = &cobra.Command{
		Use:       r.use,
		Short:     r.short,
		Long:      r.short + "\n\nSupported remote clouds: " + strings.Join(r.clouds, ", "),
		Args:      cobra.ExactArgs(1),
		ValidArgs: r.clouds,
		RunE: func(cmd *cobra.Command, args []string) error {
			cloud := strings.ToLower(args[0])
			h, ok := r.routes[cloud]
			if !ok {
				return fmt.Errorf("unsupported remote cloud %q, supported: %v", cloud, r.clouds)
			}
			return h(cmd.Context(), cmd.OutOrStdout())
		},
	}
	return r.cmd
}

// WriteProtoJSON is a convenience Handler that serialises a proto.Message as
// pretty-printed JSON and writes it to w.
func WriteProtoJSON(w io.Writer, msg proto.Message) error {
	data, err := protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling to JSON: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

// ProtoHandler returns a Handler that builds a proto.Message via fn and writes
// it as JSON. This is the common case for network / agentpool config commands.
func ProtoHandler(fn func(ctx context.Context) proto.Message) Handler {
	return func(ctx context.Context, w io.Writer) error {
		return WriteProtoJSON(w, fn(ctx))
	}
}
