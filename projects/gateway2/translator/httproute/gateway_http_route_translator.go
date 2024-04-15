package httproute

import (
	"context"
	"net/http"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/backendref"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TranslateGatewayHTTPRouteRules(
	ctx context.Context,
	pluginRegistry registry.PluginRegistry,
	queries query.GatewayQueries,
	gwListener gwv1.Listener,
	route gwv1.HTTPRoute,
	reporter reports.ParentRefReporter,
) []*v1.Route {
	var finalRoutes []*v1.Route
	for _, rule := range route.Spec.Rules {
		rule := rule
		if rule.Matches == nil {
			// from the spec:
			// If no matches are specified, the default is a prefix path match on “/”, which has the effect of matching every HTTP request.
			rule.Matches = []gwv1.HTTPRouteMatch{{}}
		}

		outputRoutes := translateGatewayHTTPRouteRule(
			ctx,
			pluginRegistry,
			queries,
			gwListener,
			&route,
			rule,
			reporter,
		)
		for _, outputRoute := range outputRoutes {
			// The above function will return a nil route if a matcher fails to apply plugins
			// properly. This is a signal to the caller that the route should be dropped.
			if outputRoute == nil {
				continue
			}

			finalRoutes = append(finalRoutes, outputRoute)
		}
	}
	return finalRoutes
}

func translateGatewayHTTPRouteRule(
	ctx context.Context,
	pluginRegistry registry.PluginRegistry,
	queries query.GatewayQueries,
	gwListener gwv1.Listener,
	gwroute *gwv1.HTTPRoute,
	rule gwv1.HTTPRouteRule,
	reporter reports.ParentRefReporter,
) []*v1.Route {
	routes := make([]*v1.Route, len(rule.Matches))
	for idx, match := range rule.Matches {
		match := match // pike
		outputRoute := &v1.Route{
			Matchers: []*matchers.Matcher{translateGlooMatcher(match)},
			Action:   nil,
			Options:  &v1.RouteOptions{},
		}

		if len(rule.BackendRefs) > 0 {
			setRouteAction(
				ctx,
				queries,
				gwroute,
				rule.BackendRefs,
				outputRoute,
				reporter,
			)
		}

		rtCtx := &plugins.RouteContext{
			Listener: &gwListener,
			Route:    gwroute,
			Rule:     &rule,
			Match:    &match,
			Reporter: reporter,
		}
		for _, plugin := range pluginRegistry.GetRoutePlugins() {
			err := plugin.ApplyRoutePlugin(ctx, rtCtx, outputRoute)
			if err != nil {
				contextutils.LoggerFrom(ctx).Errorf("error in RoutePlugin: %v", err)
			}
		}

		if outputRoute.GetAction() == nil {
			outputRoute.Action = &v1.Route_DirectResponseAction{
				DirectResponseAction: &v1.DirectResponseAction{
					Status: http.StatusInternalServerError,
				},
			}
		}
		outputRoute.Matchers = []*matchers.Matcher{translateGlooMatcher(match)}
		routes[idx] = outputRoute
	}
	return routes
}

func translateGlooMatcher(match gwv1.HTTPRouteMatch) *matchers.Matcher {
	// headers
	headers := make([]*matchers.HeaderMatcher, 0, len(match.Headers))
	for _, header := range match.Headers {
		h := translateGlooHeaderMatcher(header)
		if h != nil {
			headers = append(headers, h)
		}
	}

	// query params
	var queryParamMatchers []*matchers.QueryParameterMatcher
	for _, param := range match.QueryParams {
		queryParamMatchers = append(queryParamMatchers, &matchers.QueryParameterMatcher{
			Name:  string(param.Name),
			Value: param.Value,
			Regex: false,
		})
	}

	// set path
	pathType, pathValue := parsePath(match.Path)

	var methods []string
	if match.Method != nil {
		methods = []string{string(*match.Method)}
	}
	m := &matchers.Matcher{
		//CaseSensitive:   nil,
		Headers:         headers,
		QueryParameters: queryParamMatchers,
		Methods:         methods,
	}

	switch pathType {
	case gwv1.PathMatchPathPrefix:
		m.PathSpecifier = &matchers.Matcher_Prefix{
			Prefix: pathValue,
		}
	case gwv1.PathMatchExact:
		m.PathSpecifier = &matchers.Matcher_Exact{
			Exact: pathValue,
		}
	case gwv1.PathMatchRegularExpression:
		m.PathSpecifier = &matchers.Matcher_Regex{
			Regex: pathValue,
		}
	}

	return m
}

func translateGlooHeaderMatcher(header gwv1.HTTPHeaderMatch) *matchers.HeaderMatcher {
	regex := false
	if header.Type != nil && *header.Type == gwv1.HeaderMatchRegularExpression {
		regex = true
	}

	return &matchers.HeaderMatcher{
		Name:  string(header.Name),
		Value: header.Value,
		Regex: regex,
		//InvertMatch: header.InvertMatch,
	}
}

func parsePath(path *gwv1.HTTPPathMatch) (gwv1.PathMatchType, string) {
	pathType := gwv1.PathMatchPathPrefix
	pathValue := "/"
	if path != nil && path.Type != nil {
		pathType = *path.Type
	}
	if path != nil && path.Value != nil {
		pathValue = *path.Value
	}
	return pathType, pathValue
}

func setRouteAction(
	ctx context.Context,
	queries query.GatewayQueries,
	gwroute *gwv1.HTTPRoute,
	backendRefs []gwv1.HTTPBackendRef,
	outputRoute *v1.Route,
	reporter reports.ParentRefReporter,
) {
	var weightedDestinations []*v1.WeightedDestination

	for _, backendRef := range backendRefs {
		clusterName := "blackhole_cluster"
		ns := "blackhole_ns"
		obj, err := queries.GetBackendForRef(context.TODO(), queries.ObjToFrom(gwroute), &backendRef.BackendObjectReference)
		ptrClusterName := query.ProcessBackendRef(obj, err, reporter, backendRef.BackendObjectReference)
		if ptrClusterName != nil {
			clusterName = *ptrClusterName
			ns = obj.GetNamespace()
		}

		var weight *wrappers.UInt32Value
		if backendRef.Weight != nil {
			weight = &wrappers.UInt32Value{
				Value: uint32(*backendRef.Weight),
			}
		} else {
			// according to spec, default weight is 1
			weight = &wrappers.UInt32Value{
				Value: 1,
			}
		}

		var port uint32
		if backendRef.Port != nil {
			port = uint32(*backendRef.Port)
		}

		// get backend for ref - we must do it to make sure we have permissions to access it.
		// also we need the service so we can translate its name correctly.
		if backendref.RefIsService(backendRef.BackendObjectReference) {
			weightedDestinations = append(weightedDestinations, &v1.WeightedDestination{
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Kube{
						Kube: &v1.KubernetesServiceDestination{
							Ref: &core.ResourceRef{
								Name:      clusterName,
								Namespace: ns,
							},
							Port: port,
						},
					},
				},
				Weight:  weight,
				Options: nil,
			})
		} else {
			// TODO(npolshak): Add support for other types of destinations (upstreams, etc.)
			contextutils.LoggerFrom(ctx).Errorf("unsupported backend type for kind: %v and type: %v", *backendRef.BackendObjectReference.Kind, *backendRef.BackendObjectReference.Group)
		}
	}

	//TODO(revert): need to add ClusterNotFoundResponseCode: routev3.RouteAction_INTERNAL_SERVER_ERROR,

	switch len(weightedDestinations) {
	// case 0:
	//TODO: report error
	case 1:
		outputRoute.Action = &v1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{Single: weightedDestinations[0].GetDestination()},
			},
		}
	default:
		outputRoute.Action = &v1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Multi{Multi: &v1.MultiDestination{
					Destinations: weightedDestinations,
				}},
			},
		}
	}
}
