package trafficpolicy

import (
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/listenerpolicy"
)

type routeTracingIR struct {
	tracing *envoyroutev3.Tracing
}

var _ PolicySubIR = &routeTracingIR{}

func (t *routeTracingIR) Equals(other PolicySubIR) bool {
	otherTracing, ok := other.(*routeTracingIR)
	if !ok {
		return false
	}
	if t == nil && otherTracing == nil {
		return true
	}
	if t == nil || otherTracing == nil {
		return false
	}
	return proto.Equal(t.tracing, otherTracing.tracing)
}

func (t *routeTracingIR) Validate() error {
	if t == nil || t.tracing == nil {
		return nil
	}
	return t.tracing.Validate()
}

// constructRouteTracing constructs the route tracing IR from the policy specification.
func constructRouteTracing(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.Tracing == nil {
		return
	}

	rt := spec.Tracing

	// If disable is set, output tracing with all sampling rates set to 0%
	if rt.Disable != nil {
		out.tracing = &routeTracingIR{
			tracing: &envoyroutev3.Tracing{
				ClientSampling: &typev3.FractionalPercent{
					Numerator:   0,
					Denominator: typev3.FractionalPercent_HUNDRED,
				},
				RandomSampling: &typev3.FractionalPercent{
					Numerator:   0,
					Denominator: typev3.FractionalPercent_HUNDRED,
				},
				OverallSampling: &typev3.FractionalPercent{
					Numerator:   0,
					Denominator: typev3.FractionalPercent_HUNDRED,
				},
			},
		}
		return
	}

	tracing := &envoyroutev3.Tracing{}

	if rt.ClientSampling != nil {
		tracing.ClientSampling = &typev3.FractionalPercent{
			Numerator:   uint32(*rt.ClientSampling), // nolint:gosec // G115: kubebuilder validation ensures 0-100, safe for uint32
			Denominator: typev3.FractionalPercent_HUNDRED,
		}
	}
	if rt.RandomSampling != nil {
		tracing.RandomSampling = &typev3.FractionalPercent{
			Numerator:   uint32(*rt.RandomSampling), // nolint:gosec // G115: kubebuilder validation ensures 0-100, safe for uint32
			Denominator: typev3.FractionalPercent_HUNDRED,
		}
	}
	if rt.OverallSampling != nil {
		tracing.OverallSampling = &typev3.FractionalPercent{
			Numerator:   uint32(*rt.OverallSampling), // nolint:gosec // G115: kubebuilder validation ensures 0-100, safe for uint32
			Denominator: typev3.FractionalPercent_HUNDRED,
		}
	}
	if len(rt.Attributes) > 0 {
		tracing.CustomTags = listenerpolicy.ConvertCustomAttributesToCustomTags(rt.Attributes)
	}

	out.tracing = &routeTracingIR{
		tracing: tracing,
	}
}

// handleRouteTracing applies the route-level tracing configuration directly on the Envoy route.
// Unlike most TrafficPolicy fields which use typed_per_filter_config, route-level tracing
// is a first-class field on envoyroutev3.Route.
func (p *trafficPolicyPluginGwPass) handleRouteTracing(
	spec trafficPolicySpecIr,
	out *envoyroutev3.Route,
) {
	if spec.tracing == nil || spec.tracing.tracing == nil {
		return
	}
	out.Tracing = proto.Clone(spec.tracing.tracing).(*envoyroutev3.Tracing)
}
