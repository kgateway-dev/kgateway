package settings

import (
	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/settings/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.SETTINGS_COMMAND.Use,
		Aliases: constants.SETTINGS_COMMAND.Aliases,
		Short:   constants.SETTINGS_COMMAND.Short,
		Long:    constants.SETTINGS_COMMAND.Long,
	}
	flagutils.AddOutputFlag(cmd.PersistentFlags(), &opts.Top.Output)

	pflags := cmd.PersistentFlags()

	// TODO(yuval-k):
	// I would like the default name to be default, but currently the route subcommand will override default above.
	// to fix that we will need a significant refactor or the CLI. so i'll avoid that for now.
	//   pflags.StringVar(&opts.Metadata.Name, "name", "default", "name of the resource to read or write")
	//   flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)

	cmd.AddCommand(ExtAuthConfig(opts))
	cmd.AddCommand(ratelimit.RateLimitConfig(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
