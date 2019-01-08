package secret

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func CreateCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.SECRET_COMMAND.Use,
		Aliases: constants.SECRET_COMMAND.Aliases,
		Short:   "Create a secret",
		Long:    "Create a secret",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.AddCommand(awsCmd(opts))
	cmd.AddCommand(tlsCmd(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
