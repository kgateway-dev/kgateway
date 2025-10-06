package trafficpolicy

import (
	"math"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

const bufferFilterName = "envoy.filters.http.buffer"

type bufferIR struct {
	perRoute *bufferv3.BufferPerRoute
}

var _ PolicySubIR = &bufferIR{}

func (b *bufferIR) Equals(other PolicySubIR) bool {
	otherBuffer, ok := other.(*bufferIR)
	if !ok {
		return false
	}
	if b == nil || other == nil {
		return b == nil && otherBuffer == nil
	}
	return proto.Equal(b.perRoute, otherBuffer.perRoute)
}

// Validate performs validation on the buffer component
// Note: buffer validation is not needed as it's a single uint32 field
func (b *bufferIR) Validate() error { return nil }

// constructBuffer constructs the buffer policy IR from the policy specification.
func constructBuffer(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.Buffer == nil {
		return
	}

	perRoute := &bufferv3.BufferPerRoute{}

	if spec.Buffer.Disable != nil {
		// Disable the filter
		perRoute.Override = &bufferv3.BufferPerRoute_Disabled{
			Disabled: true,
		}
	} else {
		// Validate buffer size is within uint32 range
		bufferSize := spec.Buffer.MaxRequestSize.Value()
		if bufferSize < 0 || bufferSize > math.MaxUint32 {
			// Log error and use max uint32 value as fallback
			bufferSize = math.MaxUint32
		}
		perRoute.Override = &bufferv3.BufferPerRoute_Buffer{
			Buffer: &bufferv3.Buffer{
				MaxRequestBytes: &wrapperspb.UInt32Value{Value: uint32(bufferSize)}, //nolint:gosec // G115: validated above
			},
		}
	}
	out.buffer = &bufferIR{
		perRoute: perRoute,
	}
}

func (p *trafficPolicyPluginGwPass) handleBuffer(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, buffer *bufferIR) {
	if buffer == nil {
		return
	}

	// Add buffer configuration to the typed_per_filter_config for route-level override
	pCtxTypedFilterConfig.AddTypedConfig(bufferFilterName, buffer.perRoute)

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
