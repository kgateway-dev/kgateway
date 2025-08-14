package trafficpolicy

import (
	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_wellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

type corsIR struct {
	// policy is the envoy cors policy
	policy *corsv3.CorsPolicy
}

var _ PolicySubIR = &corsIR{}

func (c *corsIR) Equals(other PolicySubIR) bool {
	otherCors, ok := other.(*corsIR)
	if !ok {
		return false
	}
	if c == nil || otherCors == nil {
		return c == nil && otherCors == nil
	}
	return proto.Equal(c.policy, otherCors.policy)
}

func (c *corsIR) Validate() error {
	if c == nil || c.policy == nil {
		return nil
	}
	return c.policy.Validate()
}

// constructCORS constructs the CORS policy IR from the policy specification.
func constructCORS(in *v1alpha1.TrafficPolicy, out *trafficPolicySpecIr) error {
	spec := in.Spec.Cors
	if spec == nil {
		return nil
	}
	out.cors = &corsIR{
		policy: policy.BuildCorsPolicy(spec.HTTPCORSFilter, spec.Disable != nil),
	}
	return nil
}

func (p *trafficPolicyPluginGwPass) handleCors(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, cors *corsIR) {
	if cors == nil || cors.policy == nil {
		return
	}

	// Adds the CorsPolicy to the typed_per_filter_config.
	// Also requires Cors http_filter to be added to the filter chain.
	pCtxTypedFilterConfig.AddTypedConfig(envoy_wellknown.CORS, cors.policy)

	// Add a filter to the chain. When having a cors policy for a route we need to also have a
	// globally cors http filter in the chain otherwise it will be ignored.
	if p.corsInChain == nil {
		p.corsInChain = make(map[string]*corsv3.Cors)
	}
	if _, ok := p.corsInChain[fcn]; !ok {
		p.corsInChain[fcn] = &corsv3.Cors{}
	}
}
