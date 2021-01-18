package get

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/spf13/cobra"
)

func RateLimitConfig(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.RATELIMIT_CONFIG_COMMAND.Use,
		Aliases: constants.RATELIMIT_CONFIG_COMMAND.Aliases,
		Short:   "read a ratelimitconfig or list ratelimitconfigs in a namespace",
		Long:    "usage: glooctl get ratelimitconfig [NAME] [--namespace=namespace] [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {
			ratelimitConfigs, err := common.GetRateLimitConfigs(common.GetName(args, opts), opts)
			if err != nil {
				return err
			}
			_ = printers.PrintRateLimitConfigs(ratelimitConfigs, opts.Top.Output)
			return nil
		},
	}
	return cmd
}
