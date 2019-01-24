package add

import (
	"fmt"
	"sort"

	"github.com/solo-io/go-utils/cliutils"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/rest"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
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
			"Usage: `glooctl add route [--name virtual-service-name] [--namespace namespace] [--index=X] ...`",
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

func selectOrCreateVirtualService(opts *options.Options) (*gatewayv1.VirtualService, error) {
	vsClient := helpers.MustVirtualServiceClient()
	if opts.Metadata.Name != "" {
		if existing, err := vsClient.Read(opts.Metadata.Namespace, opts.Metadata.Name,
			clients.ReadOpts{Ctx: opts.Top.Ctx}); err == nil {
			return existing, nil
		}
	}

	for _, ns := range helpers.MustGetNamespaces() {
		vss, err := vsClient.List(ns, clients.ListOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}

		for _, vs := range vss {
			for _, domain := range vs.VirtualHost.Domains {
				if domain == "*" {
					fmt.Printf("selected virtualservice %v for route\n", vs.Metadata.Name)
					return vs, nil
				}
			}
		}
	}

	// TODO: edge case: check that a vs does not already exist with a * domain
	// no vs exist with default domain
	fmt.Printf("creating virtualservice %v with default domain *\n", opts.Metadata.Name)
	return &gatewayv1.VirtualService{
		Metadata: opts.Metadata,
		VirtualHost: &v1.VirtualHost{
			Domains: []string{"*"},
		},
	}, nil
}

func addRoute(opts *options.Options) error {
	match, err := matcherFromInput(opts.Add.Route.Matcher)
	if err != nil {
		return err
	}
	action, err := actionFromInput(opts.Add.Route)
	if err != nil {
		return err
	}
	plugins, err := pluginsFromInput(opts.Add.Route.Plugins)
	if err != nil {
		return err
	}

	v1Route := &v1.Route{
		Matcher:      match,
		Action:       action,
		RoutePlugins: plugins,
	}

	index := int(opts.Add.Route.InsertIndex)

	virtualService, err := selectOrCreateVirtualService(opts)
	if err != nil {
		return err
	}

	virtualService.VirtualHost.Routes = append(virtualService.VirtualHost.Routes, nil)
	copy(virtualService.VirtualHost.Routes[index+1:], virtualService.VirtualHost.Routes[index:])
	virtualService.VirtualHost.Routes[index] = v1Route

	out, err := helpers.MustVirtualServiceClient().Write(virtualService, clients.WriteOpts{
		Ctx:               opts.Top.Ctx,
		OverwriteExisting: true,
	})
	if err != nil {
		return err
	}

	helpers.PrintVirtualServices(gatewayv1.VirtualServiceList{out}, opts.Top.Output)
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

func actionFromInput(input options.InputRoute) (*v1.Route_RouteAction, error) {
	a := &v1.Route_RouteAction{
		RouteAction: &v1.RouteAction{},
	}
	// TODO: multi destination
	dest := input.Destination
	spec, err := destSpecFromInput(dest.DestinationSpec)
	if err != nil {
		return nil, err
	}
	a.RouteAction.Destination = &v1.RouteAction_Single{
		Single: &v1.Destination{
			Upstream:        dest.Upstream,
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
	case input.Aws.LogicalName != "":
		return &v1.DestinationSpec{
			DestinationType: &v1.DestinationSpec_Aws{
				Aws: &aws.DestinationSpec{
					LogicalName:            input.Aws.LogicalName,
					ResponseTrasnformation: input.Aws.ResponseTransformation,
				},
			},
		}, nil
	case input.Rest.FunctionName != "":
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
	return nil, nil // errors.Errorf("unimplemented destination type: %v", input.DestinationType)
}
