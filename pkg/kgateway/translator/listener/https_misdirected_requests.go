package listener

import (
	"context"
	"net/http"
	"strings"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const catchAllHostnamePattern = "*"

// Gateway API v1.5.1 currently exercises this feature over HTTP/2 only, but the
// authority-based virtual-host matching required to return 421 after TLS/SNI
// listener selection is protocol-agnostic once Envoy is handling HTTP traffic.

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

func shouldShadowHTTPSVirtualHost(currentPattern, hostPattern string, allPatterns []string) bool {
	host := representativeHostForHostnamePattern(hostPattern)
	if host == "" {
		return false
	}

	bestPattern := currentPattern
	for _, pattern := range allPatterns {
		if !hostnamePatternContains(pattern, hostPattern) {
			continue
		}
		if compareHostnamePatternsForHost(pattern, bestPattern, host) > 0 {
			bestPattern = pattern
		}
	}
	return bestPattern != currentPattern
}

func needsProtective404VirtualHost(currentPattern string, siblingPatterns []string) bool {
	host := representativeHostForHostnamePattern(currentPattern)
	if host == "" {
		return false
	}

	for _, siblingPattern := range siblingPatterns {
		if !hostnamePatternMatchesHost(siblingPattern, host) {
			continue
		}
		if compareHostnamePatternsForHost(currentPattern, siblingPattern, host) > 0 {
			return true
		}
	}
	return false
}

func buildHTTPSMisdirectedRequestVirtualHosts(
	ctx context.Context,
	parentName string,
	listener ir.Listener,
	currentPattern string,
	siblingPatterns []string,
	actualDomains map[string]struct{},
) []*ir.VirtualHost {
	virtualHosts := make([]*ir.VirtualHost, 0, len(siblingPatterns)+1)
	for _, siblingPattern := range siblingPatterns {
		if _, ok := actualDomains[siblingPattern]; ok {
			continue
		}
		virtualHosts = append(virtualHosts, newSyntheticDirectResponseVirtualHost(
			ctx,
			parentName+"~misdirected-request",
			siblingPattern,
			http.StatusMisdirectedRequest,
			listener,
		))
	}

	if currentPattern != catchAllHostnamePattern && needsProtective404VirtualHost(currentPattern, siblingPatterns) {
		if _, ok := actualDomains[currentPattern]; !ok {
			virtualHosts = append(virtualHosts, newSyntheticDirectResponseVirtualHost(
				ctx,
				parentName+"~listener-hostspace",
				currentPattern,
				http.StatusNotFound,
				listener,
			))
		}
	}

	return virtualHosts
}

func newSyntheticDirectResponseVirtualHost(
	ctx context.Context,
	parentName string,
	hostname string,
	statusCode uint32,
	listener ir.Listener,
) *ir.VirtualHost {
	return &ir.VirtualHost{
		Name:     makeVhostName(ctx, parentName, hostname),
		Hostname: hostname,
		DirectResponse: &ir.DirectResponseIR{
			StatusCode: statusCode,
		},
		ParentRef: listener,
	}
}
