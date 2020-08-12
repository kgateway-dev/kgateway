package demo

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/spf13/cobra"
)

var EmptyDemoError = eris.New("please provide a subcommand")

func RootCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.DEMO_COMMAND.Use,
		Short: constants.DEMO_COMMAND.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return EmptyDemoError
		},
	}
	cmd.AddCommand(federation(opts))
	return cmd
}
