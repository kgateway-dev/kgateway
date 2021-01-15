package get

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/spf13/cobra"
)

func AuthConfig(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.AUTH_CONFIG_COMMAND.Use,
		Aliases: constants.AUTH_CONFIG_COMMAND.Aliases,
		Short:   "read an authconfig or list authconfigs in a namespace",
		Long:    "usage: glooctl get authconfig [NAME] [--namespace=namespace] [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {
			authConfigs, err := common.GetAuthConfigs(common.GetName(args, opts), opts)
			if err != nil {
				return err
			}
			_ = printers.PrintAuthConfigs(authConfigs, opts.Top.Output)
			return nil
		},
	}
	return cmd
}
