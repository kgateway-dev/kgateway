package get

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"

	"github.com/spf13/cobra"
)

func VirtualService(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.VIRTUAL_SERVICE_COMMAND.Use,
		Aliases: constants.VIRTUAL_SERVICE_COMMAND.Aliases,
		Short:   "read a virtualservice or list virtualservices in a namespace",
		Long:    "usage: glooctl get virtualservice [NAME] [--namespace=namespace] [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {
			virtualServices, err := common.GetVirtualServices(common.GetName(args, opts), opts)
			if err != nil {
				return err
			}
			_ = printers.PrintVirtualServices(opts.Top.Ctx, virtualServices, opts.Top.Output, opts.Metadata.GetNamespace())
			return nil
		},
	}
	cmd.AddCommand(Routes(opts))
	return cmd
}

func Routes(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "route",
		Aliases: []string{"r", "routes"},
		Short:   "get a list of routes for a given virtual service",
		Long:    "usage: glooctl get virtualservice route [virtual service name]",
		RunE: func(cmd *cobra.Command, args []string) error {
			var vsName string
			if len(args) > 0 {
				vsName = args[0]
			}
			virtualServices, err := common.GetVirtualServices(vsName, opts)
			if err != nil {
				return err
			}
			if len(virtualServices.Names()) != 1 {
				return eris.Errorf("no virtualservice id provided")
			}
			vs, err := virtualServices.Find(opts.Metadata.GetNamespace(), opts.Metadata.GetName())
			if err != nil {
				return eris.Errorf("virtualservice id provided was incorrect")
			}
			_ = printers.PrintRoutes(vs.GetVirtualHost().GetRoutes(), opts.Top.Output)
			return nil
		},
	}
	return cmd
}
