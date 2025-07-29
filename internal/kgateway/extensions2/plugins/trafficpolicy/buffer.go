package trafficpolicy

import (
	"math"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

const bufferFilterName = "envoy.filters.http.buffer"

type bufferIR struct {
	maxRequestBytes uint32
}

func (b *bufferIR) Equals(other *bufferIR) bool {
	if b == nil && other == nil {
		return true
	}
	if b == nil || other == nil {
		return false
	}

	return b.maxRequestBytes == other.maxRequestBytes
}

// applyBuffer translates the buffer spec into an envoy buffer policy and stores it in the traffic policy IR.
func applyBuffer(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.Buffer == nil {
		return
	}

	out.buffer = &bufferIR{
		maxRequestBytes: uint32(spec.Buffer.MaxRequestSize.Value()),
	}
}

func (p *trafficPolicyPluginGwPass) handleBuffer(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, buffer *bufferIR) {
	if buffer == nil {
		return
	}

	// Add buffer configuration to the typed_per_filter_config for route-level override
	bufferPerRoute := &bufferv3.BufferPerRoute{
		Override: &bufferv3.BufferPerRoute_Buffer{
			Buffer: &bufferv3.Buffer{
				MaxRequestBytes: &wrapperspb.UInt32Value{Value: buffer.maxRequestBytes},
			},
		},
	}
	pCtxTypedFilterConfig.AddTypedConfig(bufferFilterName, bufferPerRoute)

	// Add a filter to the chain. When having a buffer policy for a route we need to also have a
	// globally disabled buffer filter in the chain otherwise it will be ignored.
	if p.bufferInChain == nil {
		p.bufferInChain = make(map[string]*bufferv3.Buffer)
	}
	if _, ok := p.bufferInChain[fcn]; !ok {
		p.bufferInChain[fcn] = &bufferv3.Buffer{
			MaxRequestBytes: &wrapperspb.UInt32Value{Value: math.MaxUint32},
		}
	}
}

// need to add disabled buffer to the filter chain
// enable on route
