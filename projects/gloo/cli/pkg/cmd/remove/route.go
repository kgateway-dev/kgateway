package remove

import (
	errors "github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	"github.com/spf13/cobra"
)

func Route(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "route",
		Aliases: []string{"r", "routes"},
		Short:   "Remove a Route from a Virtual Service",
		Long: "Routes match patterns on requests and indicate the type of action to take when a proxy receives " +
			"a matching request. Requests can be broken down into their Match and Action components. " +
			"The order of routes within a Virtual Service matters. The first route in the virtual service " +
			"that matches a given request will be selected for routing. \n\n" +
			"If no virtual service is specified for this command, glooctl add route will attempt to add it to a " +
			"default virtualservice with domain '*'. if one does not exist, it will be created for you.\n\n" +
			"" +
			"Usage: `glooctl rm route [--name virtual-service-name] [--namespace namespace] [--index x]`",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if err := surveyutils.RemoveRouteFlagsInteractive(opts); err != nil {
					return err
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return removeRoute(opts)
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)
	flagutils.RemoveRouteFlags(pflags, &opts.Remove.Route)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func removeRoute(opts *options.Options) error {
	index := int(opts.Remove.Route.RemoveIndex)
	if opts.Metadata.GetName() == "" {
		return errors.Errorf("name of the target virtual service cannot be empty")
	}

	vs, err := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace()).Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(),
		clients.ReadOpts{Ctx: opts.Top.Ctx})
	if err != nil {
		return errors.Wrapf(err, "reading vs %v", opts.Metadata.Ref())
	}

	if routeCount := len(vs.GetVirtualHost().GetRoutes()); index >= routeCount {
		return errors.Errorf("%v is greater than the number of routes on %v (%v)", index, vs.GetMetadata().Ref(), routeCount)
	}

	vs.GetVirtualHost().Routes = append(vs.GetVirtualHost().GetRoutes()[:index], vs.GetVirtualHost().GetRoutes()[index+1:]...)

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
