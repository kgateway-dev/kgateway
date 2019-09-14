package add

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

const RootAddError = "please select a subcommand"

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Aliases: []string{"a"},
		Short:   "Adds configuration to a top-level Gloo resource.",
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
			return errors.Errorf(RootAddError)
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
