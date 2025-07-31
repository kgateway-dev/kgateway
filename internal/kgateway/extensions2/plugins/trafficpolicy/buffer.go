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

var _ PolicySubIR = &bufferIR{}

func (b *bufferIR) Equals(other PolicySubIR) bool {
	otherBuffer, ok := other.(*bufferIR)
	if !ok {
		return false
	}
	if b == nil && otherBuffer == nil {
		return true
	}
	if b == nil || otherBuffer == nil {
		return false
	}
	return b.maxRequestBytes == otherBuffer.maxRequestBytes
}

// Validate performs validation on the buffer component
// Note: buffer validation is not needed as it's a single uint32 field
func (b *bufferIR) Validate() error { return nil }

// constructBuffer constructs the buffer policy IR from the policy specification.
func constructBuffer(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) {
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
