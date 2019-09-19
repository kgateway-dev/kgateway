package edit

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/route"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/settings"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/upstream"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/virtualservice"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	editFlags := &editOptions.EditOptions{Options: opts}
	return RootCmdWithEditOpts(editFlags, optionsFunc...)
}

func RootCmdWithEditOpts(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.EDIT_COMMAND.Use,
		Aliases: constants.EDIT_COMMAND.Aliases,
		Short:   constants.EDIT_COMMAND.Short,
		Long:    constants.EDIT_COMMAND.Long,

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := argsutils.MetadataArgsParse(opts.Options, args)
			if err != nil {
				return err
			}
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			if err := prerun.EnableConsulClients(opts.Edit.Consul); err != nil {
				return err
			}
			return nil
		},
	}
	flagutils.AddOutputFlag(cmd.PersistentFlags(), &opts.Top.Output)
	flagutils.AddMetadataFlags(cmd.PersistentFlags(), &opts.Metadata)

	// add resource version flag. this is not needed in interactive mode, as we can do an edit
	// atomically in that case
	addEditFlags(cmd.PersistentFlags(), opts)
	flagutils.AddConsulConfigFlags(cmd.PersistentFlags(), &opts.Edit.Consul)

	cmd.AddCommand(settings.RootCmd(opts, optionsFunc...))
	cmd.AddCommand(route.RootCmd(opts, optionsFunc...))
	cmd.AddCommand(virtualservice.RootCmd(opts, optionsFunc...))
	cmd.AddCommand(upstream.RootCmd(opts, optionsFunc...))
	return cmd
}

func addEditFlags(set *pflag.FlagSet, opts *editOptions.EditOptions) {
	set.StringVarP(&opts.ResourceVersion, "resource-version", "", "", "the resource version of the resource we are editing. if not empty, resource will only be changed if the resource version matches")
}
