package trafficpolicy

import (
	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
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
	out.cors = &CorsIR{
		corsConfig: krtcollections.ToEnvoyCorsPolicy(spec.Cors.HTTPCORSFilter),
	}
	return nil
}
