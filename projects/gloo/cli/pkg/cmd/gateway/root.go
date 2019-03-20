package gateway

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.PROXY_COMMAND.Use,
		Aliases: constants.PROXY_COMMAND.Aliases,
		Short:   "interact with proxy instances managed by Gloo",
		Long:    "these commands can be used to interact directly with the Proxies Gloo is managing. They are useful for interacting with and debugging the proxies (Envoy instances) directly.",
	}
	cmd.PersistentFlags().StringVarP(&opts.Proxy.Name, "name", "n", "gateway-proxy", "the name of the proxy service/deployment to use")
	cmd.PersistentFlags().StringVarP(&opts.Proxy.Port, "port", "p", "http", "the name of the service port to connect to")

	flagutils.AddNamespaceFlag(cmd.PersistentFlags(), &opts.Metadata.Namespace)

	cmd.AddCommand(urlCmd(opts))
	cmd.AddCommand(dumpCmd(opts))
	cmd.AddCommand(logsCmd(opts))
	cmd.AddCommand(statsCmd(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
