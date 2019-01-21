package gateway

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.PROXY_COMMAND.Use,
		Aliases: constants.PROXY_COMMAND.Aliases,
		Short:   "interact with proxy instances managed by Gloo",
	}
	cmd.PersistentFlags().StringVarP(&opts.Proxy.Name, "name", "n", "gateway-proxy", "the name of the proxy service/deployment to use")
	cmd.PersistentFlags().StringVar(&opts.Proxy.Port, "port", "http", "the name of the service port to connect to")

	cmd.AddCommand(urlCmd(opts))
	cmd.AddCommand(dumpCmd(opts))
	cmd.AddCommand(logsCmd(opts))
	cmd.AddCommand(statsCmd(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
