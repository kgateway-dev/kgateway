package policy

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoycorsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/regexutils"
)

const (
	corsFilterEnabledKey = "envoy.cors.filter_enabled"
)

// BuildCorsPolicy converts a Gateway API CORS filter to an Envoy CORS policy
func BuildCorsPolicy(
	f *gwv1.HTTPCORSFilter,
	disable bool,
) *envoycorsv3.CorsPolicy {
	if disable {
		return &envoycorsv3.CorsPolicy{
			FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
				DefaultValue: &envoytypev3.FractionalPercent{
					Numerator:   0,
					Denominator: envoytypev3.FractionalPercent_HUNDRED,
				},
				RuntimeKey: corsFilterEnabledKey,
			},
		}
	} else if f == nil {
		return nil
	}

	corsPolicy := &envoycorsv3.CorsPolicy{}
	if len(f.AllowOrigins) > 0 {
		origins := make([]*envoymatcherv3.StringMatcher, 0, len(f.AllowOrigins))
		for _, origin := range f.AllowOrigins {
			matcher := ConvertOriginToEnvoyStringMatcher(string(origin))
			if matcher != nil {
				origins = append(origins, matcher)
			}
		}
		if len(origins) > 0 {
			corsPolicy.AllowOriginStringMatch = origins
		}
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
// This implementation adheres to the definition above (from the gateway-api spec) and therefore
// an allowed origin of "https://*.example.com" will not match "https://example.com".
//
// Matching strategy:
// - No wildcard -> Exact match
// - Single wildcard at the end (e.g., "https://*") -> Prefix match
// - Any other wildcard pattern -> Regex match where * becomes .*
//
// Examples:
// - "https://example.com" -> Exact match
// - "https://*.example.com" -> Regex match: ^https://.*\.example\.com$
// - "https://example.*" -> Prefix match: "https://example."
// - "https://*" -> Prefix match: "https://"
// - "https://sub.*.example.com" -> Regex match: ^https://sub\..*\.example\.com$
// - "https://example.*:8080" -> Regex match: ^https://example\..*:8080$
func ConvertOriginToEnvoyStringMatcher(origin string) *envoymatcherv3.StringMatcher {
	// Check if the origin contains wildcards
	if !strings.Contains(origin, "*") {
		// No wildcards, use exact match
		return &envoymatcherv3.StringMatcher{
			MatchPattern: &envoymatcherv3.StringMatcher_Exact{
				Exact: origin,
			},
		}
	}

	// Check if there is a single wildcard and it is at the end
	// In this case, we can use prefix matching
	if strings.Count(origin, "*") == 1 && strings.HasSuffix(origin, "*") {
		// Extract the prefix before the wildcard
		prefix := strings.TrimSuffix(origin, "*")
		return &envoymatcherv3.StringMatcher{
			MatchPattern: &envoymatcherv3.StringMatcher_Prefix{
				Prefix: prefix,
			},
		}
	}

	// For any other wildcard pattern, use regex matching

	// First escape all special characters
	regexPattern := regexp.QuoteMeta(origin)

	// Then convert escaped wildcards to regex wildcard patterns
	regexPattern = strings.ReplaceAll(regexPattern, "\\*", ".*")

	// Test the regex pattern to make sure it is a valid RE2 pattern
	if err := regexutils.CheckRegexString(regexPattern); err != nil {
		slog.Error("failed to convert origin to regex pattern", "origin", origin, "error", err)
		return nil
	}

	return &envoymatcherv3.StringMatcher{
		MatchPattern: &envoymatcherv3.StringMatcher_SafeRegex{
			SafeRegex: &envoymatcherv3.RegexMatcher{
				EngineType: &envoymatcherv3.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoymatcherv3.RegexMatcher_GoogleRE2{},
				},
				Regex: "^" + regexPattern + "$",
			},
		},
	}
}
