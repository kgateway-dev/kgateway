package get

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func Upstream(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.UPSTREAM_COMMAND.Use,
		Aliases: constants.UPSTREAM_COMMAND.Aliases,
		Short:   "read an upstream or list upstreams in a namespace",
		Long:    "usage: glooctl get upstream [NAME] [--namespace=namespace] [-o FORMAT] [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {
			upstreams, err := common.GetUpstreams(common.GetName(args, opts), opts)
			if err != nil {
				return err
			}
			helpers.PrintUpstreams(upstreams, opts.Top.Output)
			return nil
		},
	}
	return cmd
}
