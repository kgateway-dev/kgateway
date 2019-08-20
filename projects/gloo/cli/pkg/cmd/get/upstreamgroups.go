package get

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/spf13/cobra"
)

func UpstreamGroup(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.UPSTREAM_GROUP_COMMAND.Use,
		Aliases: constants.UPSTREAM_GROUP_COMMAND.Aliases,
		Short:   "read an upstream group or list upstream groups in a namespace",
		Long:    "usage: glooctl get upstreamgroup [NAME] [--namespace=namespace] [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {
			upstreamGroups, err := common.GetUpstreamGroups(common.GetName(args, opts), opts)
			if err != nil {
				return err
			}
			_ = printers.PrintUpstreamGroups(upstreamGroups, opts.Top.Output)
			return nil
		},
	}
	return cmd
}
