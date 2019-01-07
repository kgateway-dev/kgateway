package gateway

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func Cmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "gateway",
		Aliases: []string{"gw"},
		Short:   "interact with the Gloo Gateway/Ingress",
	}
	cmd.PersistentFlags().StringVarP(&opts.Gateway.Proxy, "proxy", "p", "gateway-proxy", "the proxy to target with this command")

	cmd.AddCommand(urlCmd(opts))
	cmd.AddCommand(dumpCmd(opts))
	cmd.AddCommand(logsCmd(opts))
	cmd.AddCommand(statsCmd(opts))
	return cmd
}
