package del

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DELETE_COMMAND.Use,
		Aliases: constants.DELETE_COMMAND.Aliases,
		Short:   constants.DELETE_COMMAND.Short,
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	cmd.AddCommand(Upstream(opts))
	cmd.AddCommand(VirtualService(opts))
	cmd.AddCommand(Proxy(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
