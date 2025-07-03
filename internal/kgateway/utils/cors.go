package utils

import (
	"fmt"
	"strings"

	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ToEnvoyCorsPolicy converts a Gateway API CORS filter to an Envoy CORS policy
func ToEnvoyCorsPolicy(f *gwv1.HTTPCORSFilter) *corsv3.CorsPolicy {
	if f == nil {
		return nil
	}
	corsPolicy := &corsv3.CorsPolicy{}
	if len(f.AllowOrigins) > 0 {
		origins := make([]*envoy_type_matcher_v3.StringMatcher, len(f.AllowOrigins))
		for i, origin := range f.AllowOrigins {
			origins[i] = ConvertOriginToEnvoyStringMatcher(string(origin))
		}
		corsPolicy.AllowOriginStringMatch = origins
	}
	if len(f.AllowMethods) > 0 {
		methods := make([]string, len(f.AllowMethods))
		for i, method := range f.AllowMethods {
			methods[i] = string(method)
		}
		corsPolicy.AllowMethods = strings.Join(methods, ", ")
	}
	if len(f.AllowHeaders) > 0 {
		headers := make([]string, len(f.AllowHeaders))
		for i, header := range f.AllowHeaders {
			headers[i] = string(header)
		}
		corsPolicy.AllowHeaders = strings.Join(headers, ", ")
	}
	if f.AllowCredentials {
		corsPolicy.AllowCredentials = &wrapperspb.BoolValue{Value: bool(f.AllowCredentials)}
	}
	if len(f.ExposeHeaders) > 0 {
		headers := make([]string, len(f.ExposeHeaders))
		for i, header := range f.ExposeHeaders {
			headers[i] = string(header)
		}
		corsPolicy.ExposeHeaders = strings.Join(headers, ", ")
	}
	if f.MaxAge != 0 {
		corsPolicy.MaxAge = fmt.Sprintf("%d", f.MaxAge)
	}
	return corsPolicy
}

// ConvertOriginToEnvoyStringMatcher converts an AllowOrigins value to an Envoy StringMatcher
// based on the wildcard patterns in the origin string.
//
// The AllowOrigins format is: <scheme>://<host>(:<port>)
// The host part can contain wildcard characters '*' that behave as greedy matches to the left.
// According to the CORS specification, '*' is a greedy match to the left, including any number
// of DNS labels to the left of its position.
//
// Matching strategy:
// - No wildcard -> Exact match
// - Wildcard at the end (e.g., "example.*") -> Prefix match
// - Wildcard by itself ("*") -> Prefix match for scheme
// - Any other wildcard position -> Regex match with proper DNS label validation
//
// Examples:
// - "https://example.com" -> Exact match
// - "https://*.example.com" -> Regex match (matches example.com, www.example.com, a.b.example.com, etc.)
// - "https://example.*" -> Prefix match (matches example.com, example.org, etc.)
// - "https://*" -> Prefix match
// - "https://sub.*.example.com" -> Regex match (matches sub.example.com, sub.api.example.com, etc.)
func ConvertOriginToEnvoyStringMatcher(origin string) *envoy_type_matcher_v3.StringMatcher {
	// Check if the origin contains wildcards
	if !strings.Contains(origin, "*") {
		// No wildcards, use exact match
		return &envoy_type_matcher_v3.StringMatcher{
			MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
				Exact: origin,
			},
		}
	}

	// Check if wildcard is at the end
	if strings.HasSuffix(origin, "*") {
		// Extract the prefix before the wildcard
		prefix := strings.TrimSuffix(origin, "*")
		return &envoy_type_matcher_v3.StringMatcher{
			MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
				Prefix: prefix,
			},
		}
	}

	// For any other wildcard pattern, use regex matching

	// Replace wildcards with DNS label pattern: one or more valid DNS labels
	// DNS labels can contain: a-z, A-Z, 0-9, hyphen, underscore
	regexPattern := strings.ReplaceAll(origin, "*", "([a-zA-Z0-9_-]+.)+")

	return &envoy_type_matcher_v3.StringMatcher{
		MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
			SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
				EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
				},
				Regex: "^" + regexPattern + "$",
			},
		},
	}
}
