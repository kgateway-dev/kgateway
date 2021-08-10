package add

import (
	"sort"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"

	"github.com/solo-io/gloo/pkg/utils/selectionutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/rest"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
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
			"default virtual service with domain '*'. if one does not exist, it will be created for you.\n\n" +
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
		Matchers: []*matchers.Matcher{match}, // currently we only support adding a single matcher via glooctl
		Options:  plugins,
	}

	if opts.Add.Route.Destination.Delegate.Single.GetName() != "" {
		v1Route.Action = &gatewayv1.Route_DelegateAction{
			DelegateAction: &gatewayv1.DelegateAction{
				DelegationType: &gatewayv1.DelegateAction_Ref{
					Ref: &opts.Add.Route.Destination.Delegate.Single,
				},
			},
		}
	} else {
		v1Route.Action, err = routeActionFromInput(opts.Add.Route)
		if err != nil {
			return err
		}
	}

	if opts.Add.Route.AddToRouteTable {
		rtRef := &core.ResourceRef{
			Namespace: opts.Metadata.GetNamespace(),
			Name:      opts.Metadata.GetName(),
		}
		selector := selectionutils.NewRouteTableSelector(helpers.MustNamespacedRouteTableClient(opts.Top.Ctx, opts.Metadata.GetNamespace()), defaults.GlooSystem)
		routeTable, err := selector.SelectOrBuildRouteTable(opts.Top.Ctx, rtRef)
		if err != nil {
			return err
		}

		index := int(opts.Add.Route.InsertIndex)
		routeTable.Routes = append(routeTable.GetRoutes(), nil)
		copy(routeTable.GetRoutes()[index+1:], routeTable.GetRoutes()[index:])
		routeTable.GetRoutes()[index] = v1Route

		if !opts.Add.DryRun {
			routeTable, err = helpers.MustNamespacedRouteTableClient(opts.Top.Ctx, opts.Metadata.GetNamespace()).Write(routeTable, clients.WriteOpts{
				Ctx:               opts.Top.Ctx,
				OverwriteExisting: true,
			})
			if err != nil {
				return err
			}
			contextutils.LoggerFrom(opts.Top.Ctx).Infow("Created new default route table", zap.Any("routeTable", routeTable))
		}

		_ = printers.PrintRouteTables(gatewayv1.RouteTableList{routeTable}, opts.Top.Output)
		return nil
	}

	vsRef := &core.ResourceRef{
		Namespace: opts.Metadata.GetNamespace(),
		Name:      opts.Metadata.GetName(),
	}
	vsClient := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	nsLister := helpers.NewProvidedNamespaceLister([]string{opts.Metadata.GetNamespace()})
	if opts.Add.Route.ClusterScopedVsClient {
		vsClient = helpers.MustVirtualServiceClient(opts.Top.Ctx)
		nsLister = helpers.NewNamespaceLister()
	}
	selector := selectionutils.NewVirtualServiceSelector(vsClient, nsLister, defaults.GlooSystem)
	virtualService, err := selector.SelectOrBuildVirtualService(opts.Top.Ctx, vsRef)
	if err != nil {
		return err
	}

	index := int(opts.Add.Route.InsertIndex)
	virtualService.GetVirtualHost().Routes = append(virtualService.GetVirtualHost().GetRoutes(), nil)
	copy(virtualService.GetVirtualHost().GetRoutes()[index+1:], virtualService.GetVirtualHost().GetRoutes()[index:])
	virtualService.GetVirtualHost().GetRoutes()[index] = v1Route

	if !opts.Add.DryRun {
		virtualService, err = vsClient.Write(virtualService, clients.WriteOpts{
			Ctx:               opts.Top.Ctx,
			OverwriteExisting: true,
		})
		if err != nil {
			return err
		}
		contextutils.LoggerFrom(opts.Top.Ctx).Infow("Created new default virtual service", zap.Any("virtualService", virtualService))
	}

	_ = printers.PrintVirtualServices(opts.Top.Ctx, gatewayv1.VirtualServiceList{virtualService}, opts.Top.Output, opts.Metadata.GetNamespace())
	return nil
}

func matcherFromInput(input options.RouteMatchers) (*matchers.Matcher, error) {
	m := &matchers.Matcher{}
	switch {
	case input.PathExact != "":
		if input.PathRegex != "" || input.PathPrefix != "" {
			return nil, errors.Errorf("can only set one of path-regex, path-prefix, or path-exact")
		}
		m.PathSpecifier = &matchers.Matcher_Exact{
			Exact: input.PathExact,
		}
	case input.PathRegex != "":
		if input.PathExact != "" || input.PathPrefix != "" {
			return nil, errors.Errorf("can only set one of path-regex, path-prefix, or path-exact")
		}
		m.PathSpecifier = &matchers.Matcher_Regex{
			Regex: input.PathRegex,
		}
	case input.PathPrefix != "":
		if input.PathExact != "" || input.PathRegex != "" {
			return nil, errors.Errorf("can only set one of path-regex, path-prefix, or path-exact")
		}
		m.PathSpecifier = &matchers.Matcher_Prefix{
			Prefix: input.PathPrefix,
		}
	default:
		return nil, errors.Errorf("must provide path prefix, path exact, or path regex for route matcher")
	}
	for k, v := range input.QueryParameterMatcher.MustMap() {
		m.QueryParameters = append(m.GetQueryParameters(), &matchers.QueryParameterMatcher{
			Name:  k,
			Value: v,
			Regex: true,
		})
	}
	sort.SliceStable(m.GetQueryParameters(), func(i, j int) bool {
		return m.GetQueryParameters()[i].GetName() < m.GetQueryParameters()[j].GetName()
	})
	if len(input.Methods) > 0 {
		m.Methods = input.Methods
	}
	for k, v := range input.HeaderMatcher.MustMap() {
		m.Headers = append(m.GetHeaders(), &matchers.HeaderMatcher{
			Name:  k,
			Value: v,
			Regex: true,
		})
	}
	sort.SliceStable(m.GetHeaders(), func(i, j int) bool {
		return m.GetHeaders()[i].GetName() < m.GetHeaders()[j].GetName()
	})
	return m, nil
}

func routeActionFromInput(input options.InputRoute) (*gatewayv1.Route_RouteAction, error) {
	a := &gatewayv1.Route_RouteAction{
		RouteAction: &v1.RouteAction{},
	}

	if input.UpstreamGroup.GetName() != "" {
		if input.UpstreamGroup.GetNamespace() == "" {
			input.UpstreamGroup.Namespace = defaults.GlooSystem
		}

		a.RouteAction.Destination = &v1.RouteAction_UpstreamGroup{
			UpstreamGroup: &input.UpstreamGroup,
		}
		return a, nil
	}

	// TODO: multi destination
	dest := input.Destination
	if dest.Upstream.GetName() == "" {
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

func pluginsFromInput(input options.RoutePlugins) (*v1.RouteOptions, error) {
	if input.PrefixRewrite.Value == nil {
		return nil, nil
	}
	return &v1.RouteOptions{
		PrefixRewrite: &wrappers.StringValue{Value: *input.PrefixRewrite.Value},
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
