package trafficpolicy

import (
	"strings"

	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

type CorsIR struct {
	// corsConfig is the envoy cors policy
	corsConfig *corsv3.CorsPolicy
}

func (c *CorsIR) Equals(other *CorsIR) bool {
	if c == nil && other == nil {
		return true
	}
	if c == nil || other == nil {
		return false
	}

	return proto.Equal(c.corsConfig, other.corsConfig)
}

// corsForSpec translates the cors spec into an envoy cors policy and stores it in the traffic policy IR
func corsForSpec(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) error {
	if spec.Cors == nil {
		return nil
	}
	corsConfig, err := toCorsFilterConfig(spec.Cors)
	if err != nil {
		return err
	}
	out.cors = &CorsIR{
		corsConfig: corsConfig,
	}
	return nil
}

func toCorsFilterConfig(t *v1alpha1.CorsPolicy) (*corsv3.CorsPolicy, error) {
	if t == nil {
		return nil, nil
	}

	/*
		Envoy CORS configuration example:

			allow_origin_string_match:
				- exact: "https://example.com"
			allow_methods: "GET, POST, OPTIONS"
			allow_headers: "Content-Type, Authorization"
			max_age: "86400"
			expose_headers: "X-Custom-Header"
			allow_credentials: true
	*/

	corsPolicy := &corsv3.CorsPolicy{}
	if len(t.AllowOrigins) > 0 {
		origins := make([]*matcherv3.StringMatcher, len(t.AllowOrigins))
		for i, origin := range t.AllowOrigins {
			origins[i] = &matcherv3.StringMatcher{
				MatchPattern: &matcherv3.StringMatcher_Exact{
					Exact: origin,
				},
			}
		}
		corsPolicy.AllowOriginStringMatch = origins
	}
	if len(t.AllowMethods) > 0 {
		corsPolicy.AllowMethods = strings.Join(t.AllowMethods, ", ")
	}
	if len(t.AllowHeaders) > 0 {
		corsPolicy.AllowHeaders = strings.Join(t.AllowHeaders, ", ")
	}
	if t.AllowCredentials {
		corsPolicy.AllowCredentials = &wrappers.BoolValue{Value: t.AllowCredentials}
	}
	if len(t.ExposeHeaders) > 0 {
		corsPolicy.ExposeHeaders = strings.Join(t.ExposeHeaders, ", ")
	}
	if t.MaxAge != "" {
		corsPolicy.MaxAge = t.MaxAge
	}

	return corsPolicy, nil
}
