package trafficpolicy

import (
	faultcommonfaultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"
	faulthttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const faultFilterName = "envoy.filters.http.fault"

type faultInjectionIR struct {
	httpFault *faulthttpv3.HTTPFault
}

var _ PolicySubIR = &faultInjectionIR{}

func (f *faultInjectionIR) Equals(other PolicySubIR) bool {
	otherFault, ok := other.(*faultInjectionIR)
	if !ok {
		return false
	}
	if f == nil || otherFault == nil {
		return f == nil && otherFault == nil
	}
	return proto.Equal(f.httpFault, otherFault.httpFault)
}

func (f *faultInjectionIR) Validate() error { return nil }

func constructFaultInjection(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.FaultInjection == nil {
		return
	}

	fi := spec.FaultInjection

	// Handle disable case
	if fi.Disable != nil {
		out.faultInjection = &faultInjectionIR{
			httpFault: nil,
		}
		return
	}

	httpFault := &faulthttpv3.HTTPFault{}

	if fi.Delay != nil {
		httpFault.Delay = &faultcommonfaultv3.FaultDelay{
			FaultDelaySecifier: &faultcommonfaultv3.FaultDelay_FixedDelay{
				FixedDelay: durationpb.New(fi.Delay.FixedDelay.Duration),
			},
			Percentage: toFractionalPercent(fi.Delay.Percentage),
		}
	}

	if fi.Abort != nil {
		abort := &faulthttpv3.FaultAbort{
			Percentage: toFractionalPercent(fi.Abort.Percentage),
		}
		if fi.Abort.HttpStatus != nil {
			abort.ErrorType = &faulthttpv3.FaultAbort_HttpStatus{
				HttpStatus: uint32(max(*fi.Abort.HttpStatus, 0)), //#nosec G115 - CRD validates Minimum=200
			}
		} else if fi.Abort.GrpcStatus != nil {
			abort.ErrorType = &faulthttpv3.FaultAbort_GrpcStatus{
				GrpcStatus: uint32(max(*fi.Abort.GrpcStatus, 0)), //#nosec G115 - CRD validates Minimum=0
			}
		}
		httpFault.Abort = abort
	}

	if fi.ResponseRateLimit != nil {
		httpFault.ResponseRateLimit = &faultcommonfaultv3.FaultRateLimit{
			LimitType: &faultcommonfaultv3.FaultRateLimit_FixedLimit_{
				FixedLimit: &faultcommonfaultv3.FaultRateLimit_FixedLimit{
					LimitKbps: fi.ResponseRateLimit.KbitsPerSecond,
				},
			},
			Percentage: toFractionalPercent(fi.ResponseRateLimit.Percentage),
		}
	}

	if fi.MaxActiveFaults != nil {
		httpFault.MaxActiveFaults = &wrapperspb.UInt32Value{Value: *fi.MaxActiveFaults}
	}

	out.faultInjection = &faultInjectionIR{
		httpFault: httpFault,
	}
}

func (p *trafficPolicyPluginGwPass) handleFaultInjection(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, fi *faultInjectionIR) {
	if fi == nil {
		return
	}

	if fi.httpFault != nil {
		pCtxTypedFilterConfig.AddTypedConfig(faultFilterName, fi.httpFault)
	} else {
		// When httpFault is nil, the policy has disable set. Add an empty
		// HTTPFault as per-route config to override any fault injection
		// configured at a higher level (e.g. Gateway-attached policy).
		pCtxTypedFilterConfig.AddTypedConfig(faultFilterName, &faulthttpv3.HTTPFault{})
	}

	if p.faultInChain == nil {
		p.faultInChain = make(map[string]*faulthttpv3.HTTPFault)
	}
	if _, ok := p.faultInChain[fcn]; !ok {
		p.faultInChain[fcn] = &faulthttpv3.HTTPFault{}
	}
}

func toFractionalPercent(percentage *int32) *typev3.FractionalPercent {
	if percentage == nil {
		return nil
	}
	return &typev3.FractionalPercent{
		Numerator:   uint32(max(*percentage, 0)), //#nosec G115 - CRD validates Minimum=0
		Denominator: typev3.FractionalPercent_HUNDRED,
	}
}
