package httproute

import (
	"context"
	"errors"
	"net/http"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/registry"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TranslateGatewayHTTPRouteRules(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	queries query.GatewayQueries,
	route gwv1.HTTPRoute,
	reporter reports.ParentRefReporter,
) []*v1.Route {
	var outputRoutes []*v1.Route
	for _, rule := range route.Spec.Rules {
		rule := rule
		if rule.Matches == nil {
			// from the spec:
			// If no matches are specified, the default is a prefix path match on “/”, which has the effect of matching every HTTP request.
			rule.Matches = []gwv1.HTTPRouteMatch{{}}
		}

		outputRoute := translateGatewayHTTPRouteRule(
			ctx,
			plugins,
			queries,
			&route,
			rule,
			reporter,
		)
		if outputRoute != nil {
			outputRoutes = append(outputRoutes, outputRoute...)
		}
	}
	return outputRoutes
}

func translateGatewayHTTPRouteRule(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	queries query.GatewayQueries,
	gwroute *gwv1.HTTPRoute,
	rule gwv1.HTTPRouteRule,
	reporter reports.ParentRefReporter,
) []*v1.Route {

	routes := make([]*v1.Route, 0, len(rule.Matches))
	for _, match := range rule.Matches {
		outputRoute := &v1.Route{
			Matchers: []*matchers.Matcher{translateGlooMatcher(match)},
			Action:   nil,
			Options:  &v1.RouteOptions{},
		}
		if len(rule.BackendRefs) > 0 {
			setRouteAction(
				queries,
				gwroute,
				rule.BackendRefs,
				outputRoute,
				reporter,
			)
		}
		if err := applyFilters(
			ctx,
			plugins,
			rule.Filters,
			outputRoute,
		); err != nil {
			// TODO: report error
			//reporter.Route(gwroute).Err()
			// return nil
		}
		if outputRoute.Action == nil {
			// TODO: maybe? report error
			outputRoute.Action = &v1.Route_DirectResponseAction{
				DirectResponseAction: &v1.DirectResponseAction{
					Status: http.StatusInternalServerError,
				},
			}
		}
		routes = append(routes, outputRoute)
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
	queries query.GatewayQueries,
	gwroute *gwv1.HTTPRoute,
	backendRefs []gwv1.HTTPBackendRef,
	outputRoute *v1.Route,
	reporter reports.ParentRefReporter,
) {
	var weightedDestinations []*v1.WeightedDestination

	for _, backendRef := range backendRefs {
		name := "blackhole_cluster"
		ns := "blackhole_ns"
		cli, err := queries.GetBackendForRef(context.TODO(), queries.ObjToFrom(gwroute), &backendRef)
		if err != nil {
			switch {
			case errors.Is(err, query.ErrUnknownKind):
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonInvalidKind,
				})
			case errors.Is(err, query.ErrMissingReferenceGrant):
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonRefNotPermitted,
				})
			case apierrors.IsNotFound(err):
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonBackendNotFound,
				})
			default:
				// setting other errors to not found. not sure if there's a better option.
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonBackendNotFound,
				})
			}
		} else {
			var port uint32
			if backendRef.Port != nil {
				port = uint32(*backendRef.Port)
			}
			switch cli := cli.(type) {
			case *corev1.Service:
				if port == 0 {
					reporter.SetCondition(reports.HTTPRouteCondition{
						Type:   gwv1.RouteConditionResolvedRefs,
						Status: metav1.ConditionFalse,
						Reason: gwv1.RouteReasonUnsupportedValue,
					})
				} else {
					name = kubernetes.UpstreamName(cli.Namespace, cli.Name, int32(port))
					ns = cli.Namespace
				}
			default:
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonInvalidKind,
				})
			}
		}

		var weight *wrappers.UInt32Value
		if backendRef.Weight != nil {
			weight = &wrappers.UInt32Value{
				Value: uint32(*backendRef.Weight),
			}
		}

		// get backend for ref - we must do it to make sure we have permissions to access it.
		// also we need the service so we can translate its name correctly.

		weightedDestinations = append(weightedDestinations, &v1.WeightedDestination{
			Destination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Name:      name,
						Namespace: ns,
					},
				},
			},
			Weight:  weight,
			Options: nil,
		})
	}

	switch len(weightedDestinations) {
	// case 0:
	//TODO: report error
	case 1:
		outputRoute.Action = &v1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{Single: weightedDestinations[0].Destination},
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

func applyFilters(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	filters []gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
) error {
	for _, filter := range filters {
		if err := applyFilterPlugin(ctx, plugins, filter, outputRoute); err != nil {
			return err
		}
	}
	return nil
}

func applyFilterPlugin(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
) error {
	var (
		plugin filterplugins.FilterPlugin
		err    error
	)
	if filter.Type == gwv1.HTTPRouteFilterExtensionRef {
		plugin, err = plugins.GetExtensionPlugin(filter.ExtensionRef)
	} else {
		plugin, err = plugins.GetStandardPlugin(filter.Type)
	}
	if err != nil {
		return err
	}

	return plugin.ApplyFilter(
		ctx,
		filter,
		outputRoute,
	)
}
