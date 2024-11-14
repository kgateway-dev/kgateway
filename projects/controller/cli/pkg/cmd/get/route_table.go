package get

import (
	"github.com/solo-io/gloo/projects/controller/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/controller/cli/pkg/common"
	"github.com/solo-io/gloo/projects/controller/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/controller/cli/pkg/printers"
	"github.com/spf13/cobra"
)

func RouteTable(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ROUTE_TABLE_COMMAND.Use,
		Aliases: constants.ROUTE_TABLE_COMMAND.Aliases,
		Short:   "read a route table or list route tables in a namespace",
		Long:    "usage: glooctl get routetable [NAME] [--namespace=namespace] [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {
			routeTables, err := common.GetRouteTables(common.GetName(args, opts), opts)
			if err != nil {
				return err
			}
			_ = printers.PrintRouteTables(routeTables, opts.Top.Output)
			return nil
		},
	}
	return cmd
}
