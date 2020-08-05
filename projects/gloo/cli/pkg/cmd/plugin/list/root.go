package list

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.PLUGIN_LIST_COMMAND.Use,
		Short: constants.PLUGIN_LIST_COMMAND.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := PluginListOptions{
				NameOnly: true,
			}
			if err := o.Complete(cmd); err != nil {
				return err
			}
			return o.Run()
		},
	}
	flagutils.AddClusterFlags(cmd.PersistentFlags(), &opts.Cluster)
	return cmd
}
