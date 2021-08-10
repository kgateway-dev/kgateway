package get

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/xdsinspection"
	"github.com/spf13/cobra"
)

func Upstream(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.UPSTREAM_COMMAND.Use,
		Aliases: constants.UPSTREAM_COMMAND.Aliases,
		Short:   "read an upstream or list upstreams in a namespace",
		Long:    "usage: glooctl get upstream [NAME] [--namespace=namespace] [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {
			upstreams, err := common.GetUpstreams(common.GetName(args, opts), opts)
			if err != nil {
				return err
			}
			var xdsDump *xdsinspection.XdsDump
			if opts.Top.Output == printers.WIDE {
				xdsDump, err = xdsinspection.GetGlooXdsDump(opts.Top.Ctx, opts.Proxy.Name, opts.Metadata.GetNamespace(), false)
				if err != nil {
					return err
				}
			}
			return printers.PrintUpstreams(upstreams, opts.Top.Output, xdsDump)
		},
	}
	return cmd
}
