package get

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"g"},
		Short:   "Display one or a list of Gloo resources",
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)
	cmd.AddCommand(VirtualService(opts))
	cmd.AddCommand(Proxy(opts))
	cmd.AddCommand(Upstream(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
