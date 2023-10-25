package httproute

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sort"
	"strings"
)

// TranslateHTTPRoutes translates the set of gloo VirtualHosts required to produce the routes needed by a Gloo HTTP Filter Chain (Envoy HCM)
// the Routes passed in are assumed to be the entire set of HTTP routes intended to be exposed on a single HTTP Filter Chain.
func TranslateGatewayHTTPRoutes(
	parentName string,
	parentNamespace string,
	parentHost *gwv1.Hostname,
	routeTables []gwv1.HTTPRoute,
	reporter reports.Reporter,
) map[string]*v1.VirtualHost {
	routesByDomain := make(map[string][]*v1.Route)
	for _, rt := range routeTables {
		vhostDomains := mergeChildHosts(parentHost, rt.Spec.Hostnames)
		vhostRoutes := translateGatewayHTTPRouteRules(parentNamespace, rt.Spec.Rules, reporter)

		if len(vhostDomains) == 0 {
			// TODO report
			continue
		}
		if len(vhostRoutes) == 0 {
			// TODO report
			continue
		}

		for _, vhostDomain := range vhostDomains {
			routesByDomain[vhostDomain] = append(routesByDomain[vhostDomain], vhostRoutes...)
		}
	}

	vhostsForParent := make(map[string]*v1.VirtualHost)
	for vhostDomain, vhostRoutes := range routesByDomain {
		sortRoutes(vhostRoutes)
		vhostName := makeVhostName(parentName, vhostDomain)
		vhostsForParent[vhostName] = &v1.VirtualHost{
			Name:    vhostName,
			Domains: []string{vhostDomain},
			Routes:  vhostRoutes,
			Options: nil,
		}
	}

	return nil
}

func sortRoutes(routes []*v1.Route) {
	// TODO rout sorter
	sort.SliceStable(routes, func(i, j int) bool {
		return routes[i].Name < routes[j].Name
	})
}

func translateGatewayHTTPRouteRules(
	parentNamespace string,
	rules []gwv1.HTTPRouteRule,
	reporter reports.Reporter,
) []*v1.Route {
	var routes []*v1.Route
	for _, rule := range rules {
		route := translateGatewayHTTPRouteRule(parentNamespace, rule, reporter)
		if route != nil {
			routes = append(routes, route)
		}
	}
	return routes
}

func translateGatewayHTTPRouteRule(
	parentNamespace string,
	rule gwv1.HTTPRouteRule,
	reporter reports.Reporter,
) *v1.Route {
	route := &v1.Route{
		Matchers: translateGlooMatchers(rule.Matches),
		Action:   nil,
		Options:  translateGatewayGlooRouteOptions(rule),
	}
	setAction(parentNamespace, route, rule.Filters, rule.BackendRefs, reporter)
	if route.Action == nil {
		// TODO: report error
		return nil
	}
	return route
}

func translateGlooMatchers(matches []gwv1.HTTPRouteMatch) []*matchers.Matcher {

	var glooMatchers []*matchers.Matcher
	for _, match := range matches {

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
				Regex: true,
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

		glooMatchers = append(glooMatchers, m)
	}
	return nil
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

// makeVhostName computes the name of a virtual host based on the parent name and domain.
func makeVhostName(
	parentName string,
	domain string,
) string {
	// TODO is this a valid vh name?
	return parentName + "~" + domain
}

// mergeChildHosts computes the set of domains that a set of child routes should be exposed on
// based on the "most specific matches" between the parent domain and the child domains.
func mergeChildHosts(
	parentHost *gwv1.Hostname,
	childHosts []gwv1.Hostname,
) []string {
	var parentHostnameVal string
	if parentHost != nil {
		parentHostnameVal = string(*parentHost)
	}

	// No child hostnames specified: use the listener hostname if specified,
	// or else match all hostnames.
	if len(childHosts) == 0 {
		if len(parentHostnameVal) > 0 {
			return []string{parentHostnameVal}
		}

		return []string{"*"}
	}

	var hostnames []string

	for i := range childHosts {
		// TODO ensure childHostname is a valid hostname
		childHostname := string(childHosts[i])

		switch {
		// No listener hostname: use the route hostname.
		case len(parentHostnameVal) == 0:
			hostnames = append(hostnames, childHostname)

		// Listener hostname matches the route hostname: use it.
		case parentHostnameVal == childHostname:
			hostnames = append(hostnames, childHostname)

		// Listener has a wildcard hostname: check if the route hostname matches.
		case strings.HasPrefix(parentHostnameVal, "*"):
			if hostnameMatchesWildcardHostname(childHostname, parentHostnameVal) {
				hostnames = append(hostnames, childHostname)
			}

		// Route has a wildcard hostname: check if the listener hostname matches.
		case strings.HasPrefix(childHostname, "*"):
			if hostnameMatchesWildcardHostname(parentHostnameVal, childHostname) {
				hostnames = append(hostnames, parentHostnameVal)
			}

		}
	}

	return hostnames
}

// hostnameMatchesWildcardHostname returns true if hostname has the non-wildcard
// portion of wildcardHostname as a suffix, plus at least one DNS label matching the
// wildcard.
func hostnameMatchesWildcardHostname(hostname, wildcardHostname string) bool {
	if !strings.HasSuffix(hostname, strings.TrimPrefix(wildcardHostname, "*")) {
		return false
	}

	wildcardMatch := strings.TrimSuffix(hostname, strings.TrimPrefix(wildcardHostname, "*"))
	return len(wildcardMatch) > 0
}
