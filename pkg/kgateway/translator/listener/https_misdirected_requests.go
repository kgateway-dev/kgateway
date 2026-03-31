package listener

import (
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const catchAllHostnamePattern = "*"

// Gateway API v1.5.1 currently exercises this feature over HTTP/2 only, but the
// authority-based routing required to return 421 after TLS/SNI listener
// selection is protocol-agnostic once Envoy is handling HTTP traffic.

type httpsMisdirectedRequestPlan struct {
	routesByDomain map[string][]ir.HttpRouteRuleMatchIR
	residualRoutes []ir.HttpRouteRuleMatchIR
}

func siblingHTTPSListenerHostnamePatterns(currentListenerName string, samePortFilterChains []httpsFilterChain) []string {
	siblings := make([]string, 0, len(samePortFilterChains))
	seen := make(map[string]struct{}, len(samePortFilterChains))
	for _, filterChain := range samePortFilterChains {
		if filterChain.gatewayListenerName == currentListenerName {
			continue
		}
		pattern := normalizeHTTPSListenerHostnamePattern(filterChain.sniDomain)
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		siblings = append(siblings, pattern)
	}
	return siblings
}

func normalizeHTTPSListenerHostnamePattern(hostname *gwv1.Hostname) string {
	if hostname == nil || *hostname == "" {
		return catchAllHostnamePattern
	}
	return strings.ToLower(string(*hostname))
}

func normalizeHTTPSHostnamePattern(hostname string) string {
	if hostname == "" {
		return catchAllHostnamePattern
	}
	return strings.ToLower(hostname)
}

func isWildcardHostnamePattern(hostname string) bool {
	return strings.HasPrefix(hostname, "*.")
}

func isExactHostnamePattern(hostname string) bool {
	return hostname != "" && hostname != catchAllHostnamePattern && !isWildcardHostnamePattern(hostname)
}

func hostnamePatternMatchesHost(pattern, host string) bool {
	pattern = normalizeHTTPSHostnamePattern(pattern)
	host = strings.ToLower(host)

	switch {
	case pattern == catchAllHostnamePattern:
		return true
	case isWildcardHostnamePattern(pattern):
		return strings.HasSuffix(host, pattern[1:])
	default:
		return host == pattern
	}
}

func compareHostnamePatternsForHost(a, b, host string) int {
	aMatches := hostnamePatternMatchesHost(a, host)
	bMatches := hostnamePatternMatchesHost(b, host)

	switch {
	case aMatches && !bMatches:
		return 1
	case !aMatches && bMatches:
		return -1
	case !aMatches && !bMatches:
		return 0
	}

	aRank := hostnamePatternSpecificityRank(a)
	bRank := hostnamePatternSpecificityRank(b)
	if aRank != bRank {
		return aRank - bRank
	}

	aLen := len(normalizeHTTPSHostnamePattern(a))
	bLen := len(normalizeHTTPSHostnamePattern(b))
	switch {
	case aLen > bLen:
		return 1
	case aLen < bLen:
		return -1
	default:
		return 0
	}
}

func hostnamePatternSpecificityRank(hostname string) int {
	switch {
	case isExactHostnamePattern(hostname):
		return 2
	case isWildcardHostnamePattern(hostname):
		return 1
	default:
		return 0
	}
}

func representativeHostForHostnamePattern(hostname string) string {
	hostname = normalizeHTTPSHostnamePattern(hostname)
	switch {
	case hostname == catchAllHostnamePattern:
		return ""
	case isWildcardHostnamePattern(hostname):
		return "kgateway" + hostname[1:]
	default:
		return hostname
	}
}

func hostnamePatternContains(containerPattern, hostPattern string) bool {
	containerPattern = normalizeHTTPSHostnamePattern(containerPattern)
	hostPattern = normalizeHTTPSHostnamePattern(hostPattern)

	switch {
	case hostPattern == catchAllHostnamePattern:
		return containerPattern == catchAllHostnamePattern
	case containerPattern == catchAllHostnamePattern:
		return true
	case isExactHostnamePattern(hostPattern):
		return hostnamePatternMatchesHost(containerPattern, hostPattern)
	case isExactHostnamePattern(containerPattern):
		return false
	default:
		return strings.HasSuffix(hostPattern[1:], containerPattern[1:])
	}
}

func applyHTTPSMisdirectedRequestRoutes(
	parentName string,
	listener ir.Listener,
	currentPattern string,
	siblingPatterns []string,
	virtualHosts []*ir.VirtualHost,
) []*ir.VirtualHost {
	actualDomains := make([]string, 0, len(virtualHosts))
	for _, virtualHost := range virtualHosts {
		actualDomains = append(actualDomains, normalizeHTTPSHostnamePattern(virtualHost.Hostname))
	}

	plan := buildHTTPSMisdirectedRequestPlan(currentPattern, siblingPatterns, actualDomains)
	for _, virtualHost := range virtualHosts {
		domain := normalizeHTTPSHostnamePattern(virtualHost.Hostname)
		directResponseRoutes := plan.routesByDomain[domain]
		if len(directResponseRoutes) == 0 {
			continue
		}
		virtualHost.Rules = append(directResponseRoutes, virtualHost.Rules...)
	}

	if len(plan.residualRoutes) == 0 {
		return virtualHosts
	}

	return append(virtualHosts, &ir.VirtualHost{
		Name:      makeVhostName(parentName+"~misdirected-request", catchAllHostnamePattern),
		Hostname:  catchAllHostnamePattern,
		Rules:     plan.residualRoutes,
		ParentRef: listener,
	})
}

func buildHTTPSMisdirectedRequestPlan(
	currentPattern string,
	siblingPatterns []string,
	actualDomains []string,
) httpsMisdirectedRequestPlan {
	actualDomains = uniqueHostnamePatterns(actualDomains)
	plan := httpsMisdirectedRequestPlan{
		routesByDomain: make(map[string][]ir.HttpRouteRuleMatchIR, len(actualDomains)),
	}

	for _, actualDomain := range actualDomains {
		misdirectedPatterns := overlappingMisdirectedPatterns(currentPattern, actualDomain, siblingPatterns)
		if len(misdirectedPatterns) == 0 {
			continue
		}
		plan.routesByDomain[actualDomain] = newSyntheticAuthorityDirectResponseRoutes(
			misdirectedPatterns,
			http.StatusMisdirectedRequest,
		)
	}

	if actualDomainsContainPattern(actualDomains, catchAllHostnamePattern) {
		return plan
	}

	fallbackStatus := residualFallbackStatus(currentPattern, siblingPatterns)
	residualPatterns := residualMisdirectedPatterns(currentPattern, siblingPatterns, actualDomains)
	residualRoutes := make([]ir.HttpRouteRuleMatchIR, 0, len(residualPatterns)+2)

	if needsProtective404Route(currentPattern, residualPatterns, fallbackStatus == http.StatusMisdirectedRequest) &&
		!actualDomainsContainPattern(actualDomains, currentPattern) {
		residualRoutes = append(residualRoutes, newSyntheticAuthorityDirectResponseRoute(currentPattern, http.StatusNotFound))
	}

	residualRoutes = append(residualRoutes, newSyntheticAuthorityDirectResponseRoutes(residualPatterns, http.StatusMisdirectedRequest)...)
	if len(residualRoutes) == 0 && fallbackStatus != http.StatusMisdirectedRequest {
		return plan
	}

	residualRoutes = append(residualRoutes, newSyntheticCatchAllDirectResponseRoute(fallbackStatus))
	plan.residualRoutes = residualRoutes
	return plan
}

func overlappingMisdirectedPatterns(currentPattern, actualDomain string, siblingPatterns []string) []string {
	patterns := make([]string, 0, len(siblingPatterns))
	for _, siblingPattern := range siblingPatterns {
		overlapPattern, ok := intersectHostnamePatterns(actualDomain, siblingPattern)
		if !ok {
			continue
		}
		host := representativeHostForHostnamePattern(overlapPattern)
		if host == "" {
			continue
		}
		if compareHostnamePatternsForHost(siblingPattern, currentPattern, host) <= 0 {
			continue
		}
		patterns = append(patterns, overlapPattern)
	}
	return minimizeHostnamePatterns(patterns)
}

func residualMisdirectedPatterns(currentPattern string, siblingPatterns []string, actualDomains []string) []string {
	patterns := make([]string, 0, len(siblingPatterns))
	for _, siblingPattern := range siblingPatterns {
		if siblingPattern == catchAllHostnamePattern {
			continue
		}
		host := representativeHostForHostnamePattern(siblingPattern)
		if host == "" {
			continue
		}
		if compareHostnamePatternsForHost(siblingPattern, currentPattern, host) <= 0 {
			continue
		}
		if actualDomainsContainPattern(actualDomains, siblingPattern) {
			continue
		}
		patterns = append(patterns, siblingPattern)
	}
	return minimizeHostnamePatterns(patterns)
}

func residualFallbackStatus(currentPattern string, siblingPatterns []string) uint32 {
	if currentPattern == catchAllHostnamePattern {
		return http.StatusNotFound
	}
	if slices.Contains(siblingPatterns, catchAllHostnamePattern) {
		return http.StatusMisdirectedRequest
	}
	return http.StatusNotFound
}

func intersectHostnamePatterns(a, b string) (string, bool) {
	a = normalizeHTTPSHostnamePattern(a)
	b = normalizeHTTPSHostnamePattern(b)

	switch {
	case hostnamePatternContains(a, b):
		return b, true
	case hostnamePatternContains(b, a):
		return a, true
	default:
		return "", false
	}
}

func actualDomainsContainPattern(actualDomains []string, targetPattern string) bool {
	for _, actualDomain := range actualDomains {
		if hostnamePatternContains(actualDomain, targetPattern) {
			return true
		}
	}
	return false
}

func needsProtective404Route(currentPattern string, misdirectedPatterns []string, fallbackMisdirected bool) bool {
	host := representativeHostForHostnamePattern(currentPattern)
	if host == "" {
		return false
	}

	for _, misdirectedPattern := range misdirectedPatterns {
		if !hostnamePatternContains(misdirectedPattern, currentPattern) {
			continue
		}
		if compareHostnamePatternsForHost(currentPattern, misdirectedPattern, host) > 0 {
			return true
		}
	}

	return fallbackMisdirected && compareHostnamePatternsForHost(currentPattern, catchAllHostnamePattern, host) > 0
}

func minimizeHostnamePatterns(patterns []string) []string {
	patterns = uniqueHostnamePatterns(patterns)
	minimized := make([]string, 0, len(patterns))
	for _, candidate := range patterns {
		covered := false
		for _, other := range patterns {
			if candidate == other {
				continue
			}
			if hostnamePatternContains(other, candidate) {
				covered = true
				break
			}
		}
		if !covered {
			minimized = append(minimized, candidate)
		}
	}
	sortHostnamePatterns(minimized)
	return minimized
}

func uniqueHostnamePatterns(patterns []string) []string {
	unique := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		pattern = normalizeHTTPSHostnamePattern(pattern)
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		unique = append(unique, pattern)
	}
	return unique
}

func sortHostnamePatterns(patterns []string) {
	sort.Slice(patterns, func(i, j int) bool {
		iRank := hostnamePatternSpecificityRank(patterns[i])
		jRank := hostnamePatternSpecificityRank(patterns[j])
		if iRank != jRank {
			return iRank > jRank
		}
		if len(patterns[i]) != len(patterns[j]) {
			return len(patterns[i]) > len(patterns[j])
		}
		return patterns[i] < patterns[j]
	})
}

func newSyntheticAuthorityDirectResponseRoutes(patterns []string, statusCode uint32) []ir.HttpRouteRuleMatchIR {
	routes := make([]ir.HttpRouteRuleMatchIR, 0, len(patterns))
	for _, pattern := range patterns {
		routes = append(routes, newSyntheticAuthorityDirectResponseRoute(pattern, statusCode))
	}
	return routes
}

func newSyntheticAuthorityDirectResponseRoute(pattern string, statusCode uint32) ir.HttpRouteRuleMatchIR {
	regexMatch := gwv1.HeaderMatchRegularExpression
	return ir.HttpRouteRuleMatchIR{
		Match: gwv1.HTTPRouteMatch{
			Headers: []gwv1.HTTPHeaderMatch{
				{
					Name:  gwv1.HTTPHeaderName(":authority"),
					Value: authorityRegexForHostnamePattern(pattern),
					Type:  &regexMatch,
				},
			},
		},
		DirectResponse: &ir.DirectResponseIR{
			StatusCode: statusCode,
		},
	}
}

func newSyntheticCatchAllDirectResponseRoute(statusCode uint32) ir.HttpRouteRuleMatchIR {
	return ir.HttpRouteRuleMatchIR{
		DirectResponse: &ir.DirectResponseIR{
			StatusCode: statusCode,
		},
	}
}

func authorityRegexForHostnamePattern(pattern string) string {
	pattern = normalizeHTTPSHostnamePattern(pattern)
	switch {
	case pattern == catchAllHostnamePattern:
		return `(?i)^.+(?::[0-9]+)?$`
	case isWildcardHostnamePattern(pattern):
		return `(?i)^[^.]+(?:\.[^.]+)*` + regexp.QuoteMeta(pattern[1:]) + `(?::[0-9]+)?$`
	default:
		return `(?i)^` + regexp.QuoteMeta(pattern) + `(?::[0-9]+)?$`
	}
}
