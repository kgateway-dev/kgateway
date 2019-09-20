package debug

import (
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.DEBUG_COMMAND.Use,
		Short: constants.DEBUG_COMMAND.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return constants.SubcommandError
		},
	}

	cmd.AddCommand(DebugLogCmd(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func DebugLogCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DEBUG_LOG_COMMAND.Use,
		Aliases: constants.DEBUG_LOG_COMMAND.Aliases,
		Short:   constants.DEBUG_LOG_COMMAND.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return DebugLogs(opts, os.Stdout)
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)
	flagutils.AddFileFlag(cmd.PersistentFlags(), &opts.Top.File)
	flagutils.AddDebugFlags(cmd.PersistentFlags(), &opts.Top)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
