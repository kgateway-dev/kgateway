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

// TestBufferFilterRunsAfterDecompressionAndBeforeBodyReaders asserts the Buffer filter sorts
// behind request decompression - so max_request_bytes is measured against decompressed bytes - and
// ahead of every filter that reads or holds the request body. Staged after one of those, its
// per-route limit renders in the Envoy config but never takes effect.
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
					FilterStage: defaultExtProcFilterStage,
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

	assert.Equal(t, []string{
		extProcGlobalDisableFilterName,
		faultFilterName,
		decompressorFilterNameFor(kgateway.CompressionGzip),
		bufferFilterName,
		extProcFilterName("ext-proc"),
		rustformationFilterNamePrefix,
	}, names)

	bufferIdx := slices.Index(names, bufferFilterName)
	require.NotEqual(t, -1, bufferIdx)
	assert.Equal(t, filters.RelativeToStage(filters.WafStage, -2), sortedFilters[bufferIdx].Stage)
}
