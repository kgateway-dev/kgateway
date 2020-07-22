package federation

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/federation/list"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/federation/register"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/federation/unregister"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

var MissingSubcommandError = eris.New("please provide a subcommand")

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.CLUSTER_COMMAND.Use,
		Aliases: constants.CLUSTER_COMMAND.Aliases,
		Short:   constants.CLUSTER_COMMAND.Short,
		Long:    constants.CLUSTER_COMMAND.Long,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return MissingSubcommandError
		},
	}

	cmd.AddCommand(list.RootCmd(opts))
	cmd.AddCommand(register.RootCmd(opts))
	cmd.AddCommand(unregister.RootCmd(opts))

	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
