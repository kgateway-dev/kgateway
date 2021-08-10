package route

import (
	"fmt"

	errors "github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/spf13/cobra"
)

func Sort(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sort",
		Aliases: []string{"s"},
		Short:   "sort routes on an existing virtual service",
		Long: "The order of routes matters. A route is selected for a request based on the first matching route " +
			"matcher in the virtual service's list. Sort automatically sorts the routes in the virtual service" +
			"\n\n" +
			"Usage: `glooctl route sort [--name virtual-service-name] [--namespace virtual-service-namespace]`",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if err := surveyutils.SelectVirtualServiceInteractive(opts); err != nil {
					return err
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return sortRoutes(opts)
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func sortRoutes(opts *options.Options) error {
	if opts.Metadata.GetName() == "" {
		return errors.Errorf("name of the target virtual service cannot be empty")
	}

	vs, err := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace()).Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(),
		clients.ReadOpts{Ctx: opts.Top.Ctx})
	if err != nil {
		return errors.Wrapf(err, "reading vs %v", opts.Metadata.Ref())
	}

	fmt.Printf("sorting %v routes by:\n"+
		"- exact < regex < prefix \n"+
		"- longest path first \n"+
		"...\n", len(vs.GetVirtualHost().GetRoutes()))
	utils.SortGatewayRoutesByPath(vs.GetVirtualHost().GetRoutes())

	out, err := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace()).Write(vs, clients.WriteOpts{
		Ctx:               opts.Top.Ctx,
		OverwriteExisting: true,
	})
	if err != nil {
		return errors.Wrapf(err, "writing updated vs")
	}

	_ = printers.PrintVirtualServices(opts.Top.Ctx, gatewayv1.VirtualServiceList{out}, opts.Top.Output, opts.Metadata.GetNamespace())
	return nil
}
