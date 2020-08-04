package plugin

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/plugin/list"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.PLUGIN_COMMAND.Use,
		Short: constants.PLUGIN_COMMAND.Short,
		Long:  constants.PLUGIN_COMMAND.Long,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return constants.SubcommandError
		},
	}

	cmd.AddCommand(list.RootCmd(opts))

	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
