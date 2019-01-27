package route

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "route",
		Aliases: []string{"r"},
		Short:   "subcommands for interacting with routes within virtual services",
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	cmd.AddCommand(Sort(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
