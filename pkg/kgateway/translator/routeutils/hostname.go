package routeutils

import (
	"cmp"
	"strings"
)

// MostSpecificRouteHostname returns the most specific of a route's own
// hostnames that covers host, where host is a hostname produced by
// intersecting the route's hostnames with a listener hostname. It returns ""
// when the route has no hostnames, or none of them cover host.
//
// Gateway API uses the original route hostnames, not the intersected ones, to
// decide precedence between routes that intersect to the same hostname on a
// listener (GatewayRouteHostnameIntersectionPrecedence).
func MostSpecificRouteHostname(routeHostnames []string, host string) string {
	best := ""
	for _, h := range routeHostnames {
		if !hostnameCovers(h, host) {
			continue
		}
		if best == "" || CompareHostnameSpecificity(h, best) < 0 {
			best = h
		}
	}
	return best
}

// CompareHostnameSpecificity orders route hostnames by Gateway API hostname
// precedence. It returns -1 if a takes precedence over b, 1 if b takes
// precedence over a, and 0 if neither does. Precedence goes to the hostname
// with the most characters in a non-wildcard hostname, then the most
// characters overall. An empty hostname (a route without hostnames) has the
// lowest precedence.
func CompareHostnameSpecificity(a, b string) int {
	if c := cmp.Compare(nonWildcardLen(b), nonWildcardLen(a)); c != 0 {
		return c
	}
	return cmp.Compare(len(b), len(a))
}

func nonWildcardLen(hostname string) int {
	if strings.HasPrefix(hostname, "*") {
		return 0
	}
	return len(hostname)
}

// hostnameCovers reports whether routeHostname matches every hostname matched
// by host.
func hostnameCovers(routeHostname, host string) bool {
	if routeHostname == host {
		return true
	}
	suffix, ok := strings.CutPrefix(routeHostname, "*")
	return ok && strings.HasPrefix(suffix, ".") && strings.HasSuffix(host, suffix)
}
