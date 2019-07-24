package get

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GET_COMMAND.Use,
		Aliases: constants.GET_COMMAND.Aliases,
		Short:   constants.GET_COMMAND.Short,
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)
	cmd.AddCommand(VirtualService(opts))
	cmd.AddCommand(Proxy(opts))
	cmd.AddCommand(Upstream(opts))
	cmd.AddCommand(UpstreamGroup(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
