package gateway

import (
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
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
	cmd.PersistentFlags().StringVar(&opts.Proxy.Name, "name", defaults.GatewayProxyName, "the name of the proxy service/deployment to use")
	cmd.PersistentFlags().StringVar(&opts.Proxy.Port, "port", "http", "the name of the service port to connect to")

	flagutils.AddNamespaceFlag(cmd.PersistentFlags(), &opts.Metadata.Namespace)

	cmd.AddCommand(addressCmd(opts))
	cmd.AddCommand(urlCmd(opts))
	cmd.AddCommand(dumpCmd(opts))
	cmd.AddCommand(logsCmd(opts))
	cmd.AddCommand(statsCmd(opts))
	cmd.AddCommand(servedConfigCmd(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
