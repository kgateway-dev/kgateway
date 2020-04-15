package validate

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "validate",
		Aliases: []string{"v"},
		Short:   "Subcommands for validating gloo resources offline before application",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			if err := prerun.EnableConsulClients(opts); err != nil {
				return err
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)

	cmd.AddCommand(ValidateVirtualService(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
