package gateway

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GATEWAY_COMMAND.Use,
		Aliases: constants.GATEWAY_COMMAND.Aliases,
		Short:   constants.GATEWAY_COMMAND.Short,
	}
	cmd.PersistentFlags().StringVarP(&opts.Gateway.Proxy, "proxy", "p", "gateway-proxy", "the proxy to target with this command")

	cmd.AddCommand(urlCmd(opts))
	cmd.AddCommand(dumpCmd(opts))
	cmd.AddCommand(logsCmd(opts))
	cmd.AddCommand(statsCmd(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
