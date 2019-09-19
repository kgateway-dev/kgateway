package add

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ADD_COMMAND.Use,
		Aliases: constants.ADD_COMMAND.Aliases,
		Short:   constants.ADD_COMMAND.Short,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			if err := prerun.EnableConsulClients(opts.Add.Consul); err != nil {
				return err
			}
			if err := prerun.HarmonizeDryRunAndOutputFormat(opts, cmd); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return constants.SubcommandError
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	flagutils.AddDryRunFlag(pflags, &opts.Add.DryRun)

	flagutils.AddConsulConfigFlags(cmd.PersistentFlags(), &opts.Add.Consul)

	cmd.AddCommand(Route(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
