package add

import (
	"sort"

	"github.com/solo-io/gloo/pkg/utils/selectionutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/rest"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

func Route(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "route",
		Aliases: []string{"r", "routes"},
		Short:   "Add a Route to a Virtual Service",
		Long: "Routes match patterns on requests and indicate the type of action to take when a proxy receives " +
			"a matching request. Requests can be broken down into their Match and Action components. " +
			"The order of routes within a Virtual Service matters. The first route in the virtual service " +
			"that matches a given request will be selected for routing. \n\n" +
			"If no virtual service is specified for this command, glooctl add route will attempt to add it to a " +
			"default virtualservice with domain '*'. if one does not exist, it will be created for you.\n\n" +
			"" +
			"Usage: `glooctl add route [--name virtual-service-name] [--namespace namespace] [--index x] ...`",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if err := surveyutils.AddRouteFlagsInteractive(opts); err != nil {
					return err
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return addRoute(opts)
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)
	flagutils.AddRouteFlags(pflags, &opts.Add.Route)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func addRoute(opts *options.Options) error {
	match, err := matcherFromInput(opts.Add.Route.Matcher)
	if err != nil {
		return err
	}
	plugins, err := pluginsFromInput(opts.Add.Route.Plugins)
	if err != nil {
		return err
	}

	v1Route := &gatewayv1.Route{
		Matcher:      match,
		RoutePlugins: plugins,
	}

	if opts.Add.Route.Destination.Delegate.Name != "" {
		v1Route.Action = &gatewayv1.Route_DelegateAction{
			DelegateAction: &opts.Add.Route.Destination.Delegate,
		}
	} else {
		v1Route.Action, err = routeActionFromInput(opts.Add.Route)
		if err != nil {
			return err
		}
	}

	if opts.Add.Route.AddToRouteTable {
		rtRef := &core.ResourceRef{
			Namespace: opts.Metadata.Namespace,
			Name:      opts.Metadata.Name,
		}
		selector := selectionutils.NewRouteTableSelector(helpers.MustRouteTableClient(), helpers.NewNamespaceLister(), defaults.GlooSystem)
		routeTable, err := selector.SelectOrCreateRouteTable(opts.Top.Ctx, rtRef)
		if err != nil {
			return err
		}

		index := int(opts.Add.Route.InsertIndex)
		routeTable.Routes = append(routeTable.Routes, nil)
		copy(routeTable.Routes[index+1:], routeTable.Routes[index:])
		routeTable.Routes[index] = v1Route

		if !opts.Add.DryRun {
			routeTable, err = helpers.MustRouteTableClient().Write(routeTable, clients.WriteOpts{
				Ctx:               opts.Top.Ctx,
				OverwriteExisting: true,
			})
			if err != nil {
				return err
			}
		}

		_ = printers.PrintRouteTables(gatewayv1.RouteTableList{routeTable}, opts.Top.Output)
		return nil
	}

	vsRef := &core.ResourceRef{
		Namespace: opts.Metadata.Namespace,
		Name:      opts.Metadata.Name,
	}
	selector := selectionutils.NewVirtualServiceSelector(helpers.MustVirtualServiceClient(), helpers.NewNamespaceLister(), defaults.GlooSystem)
	virtualService, err := selector.SelectOrCreateVirtualService(opts.Top.Ctx, vsRef)
	if err != nil {
		return err
	}

	index := int(opts.Add.Route.InsertIndex)
	virtualService.VirtualHost.Routes = append(virtualService.VirtualHost.Routes, nil)
	copy(virtualService.VirtualHost.Routes[index+1:], virtualService.VirtualHost.Routes[index:])
	virtualService.VirtualHost.Routes[index] = v1Route

	if !opts.Add.DryRun {
		virtualService, err = helpers.MustVirtualServiceClient().Write(virtualService, clients.WriteOpts{
			Ctx:               opts.Top.Ctx,
			OverwriteExisting: true,
		})
		if err != nil {
			return err
		}
	}

	_ = printers.PrintVirtualServices(gatewayv1.VirtualServiceList{virtualService}, opts.Top.Output)
	return nil
}

func matcherFromInput(input options.RouteMatchers) (*v1.Matcher, error) {
	m := &v1.Matcher{}
	switch {
	case input.PathExact != "":
		if input.PathRegex != "" || input.PathPrefix != "" {
			return nil, errors.Errorf("can only set one of path-regex, path-prefix, or path-exact")
		}
		m.PathSpecifier = &v1.Matcher_Exact{
			Exact: input.PathExact,
		}
	case input.PathRegex != "":
		if input.PathExact != "" || input.PathPrefix != "" {
			return nil, errors.Errorf("can only set one of path-regex, path-prefix, or path-exact")
		}
		m.PathSpecifier = &v1.Matcher_Regex{
			Regex: input.PathRegex,
		}
	case input.PathPrefix != "":
		if input.PathExact != "" || input.PathRegex != "" {
			return nil, errors.Errorf("can only set one of path-regex, path-prefix, or path-exact")
		}
		m.PathSpecifier = &v1.Matcher_Prefix{
			Prefix: input.PathPrefix,
		}
	default:
		return nil, errors.Errorf("must provide path prefix, path exact, or path regex for route matcher")
	}
	for k, v := range input.QueryParameterMatcher.MustMap() {
		m.QueryParameters = append(m.QueryParameters, &v1.QueryParameterMatcher{
			Name:  k,
			Value: v,
			Regex: true,
		})
	}
	sort.SliceStable(m.QueryParameters, func(i, j int) bool {
		return m.QueryParameters[i].Name < m.QueryParameters[j].Name
	})
	if len(input.Methods) > 0 {
		m.Methods = input.Methods
	}
	for k, v := range input.HeaderMatcher.MustMap() {
		m.Headers = append(m.Headers, &v1.HeaderMatcher{
			Name:  k,
			Value: v,
			Regex: true,
		})
	}
	sort.SliceStable(m.Headers, func(i, j int) bool {
		return m.Headers[i].Name < m.Headers[j].Name
	})
	return m, nil
}

func routeActionFromInput(input options.InputRoute) (*gatewayv1.Route_RouteAction, error) {
	a := &gatewayv1.Route_RouteAction{
		RouteAction: &v1.RouteAction{},
	}

	if input.UpstreamGroup.Name != "" {
		if input.UpstreamGroup.Namespace == "" {
			input.UpstreamGroup.Namespace = defaults.GlooSystem
		}

		a.RouteAction.Destination = &v1.RouteAction_UpstreamGroup{
			UpstreamGroup: &input.UpstreamGroup,
		}
		return a, nil
	}

	// TODO: multi destination
	dest := input.Destination
	if dest.Upstream.Name == "" {
		return nil, errors.Errorf("must provide destination name")
	}
	spec, err := destSpecFromInput(dest.DestinationSpec)
	if err != nil {
		return nil, err
	}
	a.RouteAction.Destination = &v1.RouteAction_Single{
		Single: &v1.Destination{
			DestinationType: &v1.Destination_Upstream{
				Upstream: &dest.Upstream,
			},
			DestinationSpec: spec,
		},
	}

	return a, nil
}

func pluginsFromInput(input options.RoutePlugins) (*v1.RoutePlugins, error) {
	if input.PrefixRewrite.Value == nil {
		return nil, nil
	}
	return &v1.RoutePlugins{
		PrefixRewrite: &transformation.PrefixRewrite{
			PrefixRewrite: *input.PrefixRewrite.Value,
		},
	}, nil
}

func destSpecFromInput(input options.DestinationSpec) (*v1.DestinationSpec, error) {
	switch {
	case input.Aws.LogicalName != "" && input.Aws.LogicalName != surveyutils.NoneOfTheAbove:
		return &v1.DestinationSpec{
			DestinationType: &v1.DestinationSpec_Aws{
				Aws: &aws.DestinationSpec{
					LogicalName:            input.Aws.LogicalName,
					ResponseTransformation: input.Aws.ResponseTransformation,
				},
			},
		}, nil
	case input.Rest.FunctionName != "" && input.Rest.FunctionName != surveyutils.NoneOfTheAbove:
		return &v1.DestinationSpec{
			DestinationType: &v1.DestinationSpec_Rest{
				Rest: &rest.DestinationSpec{
					FunctionName: input.Rest.FunctionName,
					Parameters: &transformation.Parameters{
						Headers: input.Rest.Parameters.MustMap(),
					},
				},
			},
		}, nil
	}
	return nil, nil
}
