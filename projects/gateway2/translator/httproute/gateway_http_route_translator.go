package httproute

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TranslateGatewayHTTPRouteRules(
	parentNamespace string,
	rules []gwv1.HTTPRouteRule,
	reporter reports.Reporter,
) []*v1.Route {
	var routes []*v1.Route
	for _, rule := range rules {
		matchRoutes := translateGatewayHTTPRouteRule(parentNamespace, rule, reporter)
		routes = append(routes, matchRoutes...)
	}
	return routes
}

func translateGatewayHTTPRouteRule(
	parentNamespace string,
	rule gwv1.HTTPRouteRule,
	reporter reports.Reporter,
) []*v1.Route {

	routes := make([]*v1.Route, 0, len(rule.Matches))
	for _, match := range rule.Matches {
		route := &v1.Route{
			Matchers: []*matchers.Matcher{translateGlooMatcher(match)},
			Action:   nil,
			Options:  translateGatewayGlooRouteOptions(rule),
		}
		setAction(parentNamespace, route, rule.Filters, rule.BackendRefs, reporter)
		if route.Action == nil {
			// TODO: report error
			// return nil
		}
		routes = append(routes, route)
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
	return &matchers.HeaderMatcher{
		Name:  string(header.Name),
		Value: header.Value,
		// TODO(ilackarms) SUPPORT REGEX MATCH BY DEFAULT??
		Regex: true,
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

func setAction(
	parentNamespace string,
	route *v1.Route,
	filters []gwv1.HTTPRouteFilter,
	backendRefs []gwv1.HTTPBackendRef,
	reporter reports.Reporter,
) {
	var weightedDestinations []*v1.WeightedDestination

	for _, backendRef := range backendRefs {

		// TODO validate backend ref

		namespace := parentNamespace
		if backendRef.Namespace != nil {
			// TODO VERIFY THE REFERENCE GRANT PERMISSIONS TO THE NAMESPACE
			namespace = string(*backendRef.Namespace)
		}
		var weight *wrappers.UInt32Value
		if backendRef.Weight != nil {
			weight = &wrappers.UInt32Value{
				Value: uint32(*backendRef.Weight),
			}
		}

		var port uint32
		if backendRef.Port != nil {
			port = uint32(*backendRef.Port)
		}

		weightedDestinations = append(weightedDestinations, &v1.WeightedDestination{
			Destination: &v1.Destination{
				DestinationType: &v1.Destination_Kube{
					Kube: &v1.KubernetesServiceDestination{
						Ref: &core.ResourceRef{
							Name:      string(backendRef.Name),
							Namespace: namespace,
						},
						Port: port,
					},
				},
			},
			Weight:  weight,
			Options: nil,
		})
	}

	route.Action = &v1.Route_RouteAction{
		RouteAction: &v1.RouteAction{
			Destination: &v1.RouteAction_Multi{Multi: &v1.MultiDestination{
				Destinations: weightedDestinations,
			}},
		},
	}
}

func translateGatewayGlooRouteOptions(rule gwv1.HTTPRouteRule) *v1.RouteOptions {
	// TODO
	return nil
}
