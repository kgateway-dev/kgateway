package trafficpolicy

import (
	"slices"
	"testing"

	envoymatchingv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/matching/v3"
	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	decompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/decompressor/v3"
	faulthttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestBufferIREquals(t *testing.T) {
	tests := []struct {
		name string
		a, b *kgateway.Buffer
		want bool
	}{
		{
			name: "both nil are equal",
			want: true,
		},
		{
			name: "non-nil and not equal",
			a: &kgateway.Buffer{
				MaxRequestSize: new(resource.MustParse("1Ki")),
			},
			b: &kgateway.Buffer{
				MaxRequestSize: new(resource.MustParse("2Ki")),
			},
			want: false,
		},
		{
			name: "non-nil and equal",
			a: &kgateway.Buffer{
				MaxRequestSize: new(resource.MustParse("1Ki")),
			},
			b: &kgateway.Buffer{
				MaxRequestSize: new(resource.MustParse("1Ki")),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			aOut := &trafficPolicySpecIr{}
			constructBuffer(kgateway.TrafficPolicySpec{
				Buffer: tt.a,
			}, aOut)

			bOut := &trafficPolicySpecIr{}
			constructBuffer(kgateway.TrafficPolicySpec{
				Buffer: tt.b,
			}, bOut)

			a.Equal(tt.want, aOut.buffer.Equals(bOut.buffer))
		})
	}
}

// TestBufferFilterRunsAfterDecompressionAndBeforeBodyReaders asserts the chain invariant
// request decompression -> Buffer -> every body-reading filter, with the ext_proc provider at
// BeforeStage(FaultStage), the earliest position a GatewayExtension can ask for.
//
// Staged after a filter that reads the body, Buffer's per-route max_request_bytes renders in the
// Envoy config but never takes effect; staged ahead of decompression, it measures the request
// against its compressed size and a small compressed body expands past the limit.
func TestBufferFilterRunsAfterDecompressionAndBeforeBodyReaders(t *testing.T) {
	const filterChainName = "test-filter-chain"

	plugin := &trafficPolicyPluginGwPass{
		setTransformationInChain: map[string]bool{
			filterChainName: true,
		},
		bufferInChain: map[string]*bufferv3.Buffer{
			filterChainName: {
				MaxRequestBytes: &wrapperspb.UInt32Value{Value: 1024},
			},
		},
		faultInChain: map[string]*faulthttpv3.HTTPFault{
			filterChainName: {},
		},
		decompressorInChain: map[string][]decompressorEntry{
			filterChainName: {{
				filterName:   decompressorFilterNameFor(kgateway.CompressionGzip),
				decompressor: &decompressorv3.Decompressor{},
			}},
		},
		extProcPerProvider: ProviderNeededMap{
			Providers: map[string][]Provider{
				filterChainName: {{
					Name: "ext-proc",
					Extension: &TrafficPolicyGatewayExtensionIR{
						Name:    "ext-proc",
						ExtProc: &envoymatchingv3.ExtensionWithMatcher{},
					},
					// the earliest stage a GatewayExtension can ask for
					FilterStage: filters.BeforeStage(filters.FaultStage),
				}},
			},
		},
	}

	httpFilters, err := plugin.HttpFilters(
		ir.HttpFiltersContext{},
		ir.FilterChainCommon{FilterChainName: filterChainName},
	)
	require.NoError(t, err)

	sortedFilters := filters.StagedHttpFilterList(httpFilters)
	sortedFilters.Sort()

	names := make([]string, 0, len(sortedFilters))
	for _, f := range sortedFilters {
		names = append(names, f.Filter.GetName())
	}

	require.Equal(t, decompressorFilterNameFor(kgateway.CompressionGzip), names[0], "got %v", names)
	require.Equal(t, bufferFilterName, names[1], "got %v", names)
	assert.Equal(t, filters.RelativeToStage(filters.FaultStage, -3), sortedFilters[0].Stage)
	assert.Equal(t, filters.RelativeToStage(filters.FaultStage, -2), sortedFilters[1].Stage)

	// everything that reads or holds the request body sorts behind Buffer
	for _, name := range []string{
		extProcFilterName("ext-proc"),
		faultFilterName,
		rustformationFilterNamePrefix,
	} {
		idx := slices.Index(names, name)
		require.NotEqual(t, -1, idx, "%s missing from %v", name, names)
		assert.Greater(t, idx, 1, "%s must sort behind Buffer, got %v", name, names)
	}
}
