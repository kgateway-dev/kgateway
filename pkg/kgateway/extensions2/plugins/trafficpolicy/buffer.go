package trafficpolicy

import (
	"math"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const bufferFilterName = "envoy.filters.http.buffer"

// defaultBufferFilterStage places the buffer filter immediately ahead of the transformation
// filter, which puts it behind authentication, authorization and rate limiting: a request those
// filters reject is rejected before its body is buffered.
var defaultBufferFilterStage = filters.RelativeToStage(filters.AcceptedStage, -2)

type bufferIR struct {
	perRoute *bufferv3.BufferPerRoute
	// filterStage is where the buffer filter is placed in the chain. It is a chain-level property
	// rather than a per-route one, see bufferChainEntry.
	filterStage filters.FilterStage[filters.WellKnownFilterStage]
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
	return b.filterStage == otherBuffer.filterStage &&
		proto.Equal(b.perRoute, otherBuffer.perRoute)
}

// Validate performs validation on the buffer component
// Note: buffer validation is not needed as it's a single uint32 field
func (b *bufferIR) Validate() error { return nil }

// constructBuffer constructs the buffer policy IR from the policy specification.
func constructBuffer(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
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
		perRoute:    perRoute,
		filterStage: convertFilterStageSpec(spec.Buffer.FilterStage, defaultBufferFilterStage),
	}
}

// bufferChainEntry is the single buffer filter installed in a filter chain.
//
// Envoy resolves typed_per_filter_config by filter name, and that name-based lookup is what makes
// a route-level buffer policy override a Gateway-level one. Naming the filter after its stage
// would break that hierarchy, so a chain gets exactly one buffer filter under the well-known name
// and a single stage. When policies on the same chain disagree, the earliest requested stage wins:
// it is the placement that enforces in the most cases, and picking the extreme rather than the
// first one seen keeps the output independent of the order in which policies are visited.
type bufferChainEntry struct {
	buffer *bufferv3.Buffer
	stage  filters.FilterStage[filters.WellKnownFilterStage]
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
		p.bufferInChain = make(map[string]*bufferChainEntry)
	}
	entry, ok := p.bufferInChain[fcn]
	if !ok {
		p.bufferInChain[fcn] = &bufferChainEntry{
			buffer: &bufferv3.Buffer{
				MaxRequestBytes: &wrapperspb.UInt32Value{Value: math.MaxUint32},
			},
			stage: buffer.filterStage,
		}
		return
	}
	if filters.FilterStageComparison(buffer.filterStage, entry.stage) < 0 {
		entry.stage = buffer.filterStage
	}
}

// need to add disabled buffer to the filter chain
// enable on route
