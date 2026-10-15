package trafficpolicy

import (
	"slices"
	"testing"

	envoymatchingv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/matching/v3"
	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	decompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/decompressor/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
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
		{
			name: "same size, different filter stage",
			a: &kgateway.Buffer{
				MaxRequestSize: new(resource.MustParse("1Ki")),
			},
			b: &kgateway.Buffer{
				MaxRequestSize: new(resource.MustParse("1Ki")),
				FilterStage: &kgateway.FilterStageSpec{
					Stage:     kgateway.FilterStageAuthN,
					Predicate: kgateway.FilterStagePredicateBefore,
				},
			},
			want: false,
		},
		{
			name: "same size, same filter stage",
			a: &kgateway.Buffer{
				MaxRequestSize: new(resource.MustParse("1Ki")),
				FilterStage: &kgateway.FilterStageSpec{
					Stage:     kgateway.FilterStageAuthN,
					Predicate: kgateway.FilterStagePredicateBefore,
				},
			},
			b: &kgateway.Buffer{
				MaxRequestSize: new(resource.MustParse("1Ki")),
				FilterStage: &kgateway.FilterStageSpec{
					Stage:     kgateway.FilterStageAuthN,
					Predicate: kgateway.FilterStagePredicateBefore,
				},
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

func TestBufferFilterRunsImmediatelyBeforeRustformation(t *testing.T) {
	const filterChainName = "test-filter-chain"

	plugin := &trafficPolicyPluginGwPass{
		setTransformationInChain: map[string]bool{
			filterChainName: true,
		},
		bufferInChain: map[string]*bufferChainEntry{
			filterChainName: {
				buffer: &bufferv3.Buffer{
					MaxRequestBytes: &wrapperspb.UInt32Value{Value: 1024},
				},
				stage: &defaultBufferFilterStage,
			},
		},
	}

	httpFilters, err := plugin.HttpFilters(
		ir.HttpFiltersContext{},
		ir.FilterChainCommon{FilterChainName: filterChainName},
	)
	require.NoError(t, err)
	require.Len(t, httpFilters, 2)

	sortedFilters := filters.StagedHttpFilterList(httpFilters)
	sortedFilters.Sort()

	assert.Equal(t, bufferFilterName, sortedFilters[0].Filter.GetName())
	assert.Equal(t, filters.RelativeToStage(filters.AcceptedStage, -2), sortedFilters[0].Stage)
	assert.Equal(t, rustformationFilterNamePrefix, sortedFilters[1].Filter.GetName())
	assert.Equal(t, filters.BeforeStage(filters.AcceptedStage), sortedFilters[1].Stage)
}

func TestConstructBufferFilterStage(t *testing.T) {
	tests := []struct {
		name string
		spec *kgateway.FilterStageSpec
		want filters.FilterStage[filters.WellKnownFilterStage]
	}{
		{
			name: "unset keeps the default placement, behind authn/authz/ratelimit",
			want: defaultBufferFilterStage,
		},
		{
			name: "before authn, so the limit enforces ahead of ext_authz",
			spec: &kgateway.FilterStageSpec{
				Stage:     kgateway.FilterStageAuthN,
				Predicate: kgateway.FilterStagePredicateBefore,
			},
			want: filters.BeforeStage(filters.AuthNStage),
		},
		{
			name: "earliest position the API can express",
			spec: &kgateway.FilterStageSpec{
				Stage:     kgateway.FilterStageFault,
				Predicate: kgateway.FilterStagePredicateBefore,
			},
			want: filters.BeforeStage(filters.FaultStage),
		},
		{
			name: "predicate defaults to during",
			spec: &kgateway.FilterStageSpec{
				Stage: kgateway.FilterStageAuthZ,
			},
			want: filters.DuringStage(filters.AuthZStage),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &trafficPolicySpecIr{}
			constructBuffer(kgateway.TrafficPolicySpec{
				Buffer: &kgateway.Buffer{
					MaxRequestSize: new(resource.MustParse("1Ki")),
					FilterStage:    tt.spec,
				},
			}, out)

			require.NotNil(t, out.buffer)
			require.NotNil(t, out.buffer.filterStage)
			assert.Equal(t, tt.want, *out.buffer.filterStage)
		})
	}
}

// A configured stage moves the chain's buffer filter ahead of ext_authz, which is what makes
// maxRequestSize enforce for a route whose body ext_authz would otherwise read first.
func TestBufferFilterStageMovesBufferAheadOfExtAuth(t *testing.T) {
	const filterChainName = "test-filter-chain"

	plugin := &trafficPolicyPluginGwPass{
		extAuthPerProvider: ProviderNeededMap{
			Providers: map[string][]Provider{
				filterChainName: {{
					Name: "test-extension",
					Extension: &TrafficPolicyGatewayExtensionIR{
						Name:    "test-extension",
						ExtAuth: &envoy_ext_authz_v3.ExtAuthz{},
					},
				}},
			},
		},
	}

	pCtx := &ir.TypedFilterConfigMap{}
	plugin.handleBuffer(filterChainName, pCtx, &bufferIR{
		perRoute:    &bufferv3.BufferPerRoute{},
		filterStage: new(filters.BeforeStage(filters.AuthNStage)),
	})

	httpFilters, err := plugin.HttpFilters(
		ir.HttpFiltersContext{},
		ir.FilterChainCommon{FilterChainName: filterChainName},
	)
	require.NoError(t, err)

	sortedFilters := filters.StagedHttpFilterList(httpFilters)
	sortedFilters.Sort()
	names := filterNames(sortedFilters)

	bufferIdx := slices.Index(names, bufferFilterName)
	extAuthIdx := slices.Index(names, extAuthFilterName("test-extension"))
	require.NotEqual(t, -1, bufferIdx, "buffer missing from %v", names)
	require.NotEqual(t, -1, extAuthIdx, "ext_authz missing from %v", names)
	assert.Less(t, bufferIdx, extAuthIdx, "buffer must sort ahead of ext_authz, got %v", names)
}

// A filter chain carries a single buffer filter, since Envoy resolves the per-route override by
// filter name. When policies on the chain disagree, the earliest requested stage wins, whichever
// order they are visited in.
func TestBufferChainStageTakesTheEarliestRequested(t *testing.T) {
	const filterChainName = "test-filter-chain"

	early := &bufferIR{perRoute: &bufferv3.BufferPerRoute{}, filterStage: new(filters.BeforeStage(filters.AuthNStage))}
	late := &bufferIR{perRoute: &bufferv3.BufferPerRoute{}, filterStage: &defaultBufferFilterStage}

	for _, tc := range []struct {
		name  string
		order []*bufferIR
	}{
		{name: "early first", order: []*bufferIR{early, late}},
		{name: "late first", order: []*bufferIR{late, early}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &trafficPolicyPluginGwPass{}
			for _, b := range tc.order {
				plugin.handleBuffer(filterChainName, &ir.TypedFilterConfigMap{}, b)
			}
			require.NotNil(t, plugin.bufferInChain[filterChainName])
			assert.Equal(t, filters.BeforeStage(filters.AuthNStage), plugin.bufferInChain[filterChainName].filterStage())
		})
	}
}

// A policy that only turns buffering off for its route says nothing about where the chain's buffer
// filter belongs. Letting it vote the default into the earliest-wins fold would mean that disabling
// buffering on one route moves the buffer filter for every other route on the listener - and, since
// the default sorts ahead of anything staged at Route, moves it ahead of transformations for a
// route that deliberately asked to buffer after them.
func TestDisableOnlyPolicyDoesNotClaimAStage(t *testing.T) {
	const filterChainName = "test-filter-chain"

	buffersLate := &trafficPolicySpecIr{}
	constructBuffer(kgateway.TrafficPolicySpec{Buffer: &kgateway.Buffer{
		MaxRequestSize: new(resource.MustParse("1Ki")),
		FilterStage: &kgateway.FilterStageSpec{
			Stage:     kgateway.FilterStageRoute,
			Predicate: kgateway.FilterStagePredicateBefore,
		},
	}}, buffersLate)

	disabled := &trafficPolicySpecIr{}
	constructBuffer(kgateway.TrafficPolicySpec{Buffer: &kgateway.Buffer{
		Disable: &shared.PolicyDisable{},
	}}, disabled)

	require.Nil(t, disabled.buffer.filterStage, "a disable-only policy must not carry a placement")

	t.Run("disable does not move a stage another policy claimed", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		plugin.handleBuffer(filterChainName, &ir.TypedFilterConfigMap{}, buffersLate.buffer)
		plugin.handleBuffer(filterChainName, &ir.TypedFilterConfigMap{}, disabled.buffer)

		assert.Equal(t, filters.BeforeStage(filters.RouteStage),
			plugin.bufferInChain[filterChainName].filterStage())
	})

	t.Run("disable seen first does not consume the claim", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		plugin.handleBuffer(filterChainName, &ir.TypedFilterConfigMap{}, disabled.buffer)
		plugin.handleBuffer(filterChainName, &ir.TypedFilterConfigMap{}, buffersLate.buffer)

		assert.Equal(t, filters.BeforeStage(filters.RouteStage),
			plugin.bufferInChain[filterChainName].filterStage())
	})

	t.Run("a chain with only disables still installs the filter at the default", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{}
		plugin.handleBuffer(filterChainName, &ir.TypedFilterConfigMap{}, disabled.buffer)

		entry := plugin.bufferInChain[filterChainName]
		require.NotNil(t, entry, "the chain still needs a filter for the per-route override to land on")
		assert.Nil(t, entry.stage)
		assert.Equal(t, defaultBufferFilterStage, entry.filterStage())
	})
}

// Buffering the encoded bytes would measure the request against its compressed size, so the
// decompressors have to stay ahead of the buffer filter wherever the buffer filter is staged.
//
// wantStage pins where they land, not just that the order comes out right: AfterStage(CorsStage)
// already precedes every stage FilterStageSpec can name except Fault, so most placements leave the
// decompressors alone and only a buffer filter staged at Fault moves them.
func TestDecompressorsStayAheadOfBuffer(t *testing.T) {
	const filterChainName = "test-filter-chain"

	defaultStage := filters.AfterStage(filters.WellKnownFilterStage(filters.CorsStage))

	tests := []struct {
		name        string
		bufferStage *filters.FilterStage[filters.WellKnownFilterStage]
		wantStage   filters.FilterStage[filters.WellKnownFilterStage]
	}{
		{
			name:      "no buffer policy on the chain",
			wantStage: defaultStage,
		},
		{
			name:        "default buffer stage leaves the decompressors alone",
			bufferStage: &defaultBufferFilterStage,
			wantStage:   defaultStage,
		},
		{
			name:        "buffer before authn still leaves the decompressors alone",
			bufferStage: new(filters.BeforeStage(filters.AuthNStage)),
			wantStage:   defaultStage,
		},
		{
			name:        "buffer at the head of the chain pulls them along",
			bufferStage: new(filters.BeforeStage(filters.FaultStage)),
			wantStage:   filters.RelativeToStage(filters.FaultStage, headOfStageWeight),
		},
		{
			name:        "buffer after fault pulls them to the head of that stage",
			bufferStage: new(filters.AfterStage(filters.FaultStage)),
			wantStage:   filters.RelativeToStage(filters.FaultStage, headOfStageWeight),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &trafficPolicyPluginGwPass{
				decompressorInChain: map[string][]decompressorEntry{
					filterChainName: {{
						filterName:   decompressorFilterNameFor(kgateway.CompressionGzip),
						decompressor: &decompressorv3.Decompressor{},
					}},
				},
			}
			if tt.bufferStage != nil {
				plugin.handleBuffer(filterChainName, &ir.TypedFilterConfigMap{}, &bufferIR{
					perRoute:    &bufferv3.BufferPerRoute{},
					filterStage: tt.bufferStage,
				})
			}

			assert.Equal(t, tt.wantStage, decompressorStage(plugin, filterChainName))

			httpFilters, err := plugin.HttpFilters(
				ir.HttpFiltersContext{},
				ir.FilterChainCommon{FilterChainName: filterChainName},
			)
			require.NoError(t, err)

			sortedFilters := filters.StagedHttpFilterList(httpFilters)
			sortedFilters.Sort()
			names := filterNames(sortedFilters)

			decompressorIdx := slices.Index(names, decompressorFilterNameFor(kgateway.CompressionGzip))
			require.NotEqual(t, -1, decompressorIdx, "decompressor missing from %v", names)
			if tt.bufferStage == nil {
				return
			}
			bufferIdx := slices.Index(names, bufferFilterName)
			require.NotEqual(t, -1, bufferIdx, "buffer missing from %v", names)
			assert.Less(t, decompressorIdx, bufferIdx, "decompressor must sort ahead of buffer, got %v", names)
		})
	}
}

// The decompressors must beat a body-reading filter sharing their well-known stage on stage alone.
// CompareStagedFilters falls back to the filter name for equal stages, and the names happen to sort
// the right way today, so assert on the stage rather than on the resulting order.
func TestDecompressorsOutrankBodyReadersAtTheSameStage(t *testing.T) {
	const filterChainName = "test-filter-chain"

	plugin := &trafficPolicyPluginGwPass{
		decompressorInChain: map[string][]decompressorEntry{
			filterChainName: {{
				filterName:   decompressorFilterNameFor(kgateway.CompressionGzip),
				decompressor: &decompressorv3.Decompressor{},
			}},
		},
		// the latest placement at Fault a GatewayExtension can ask for
		extProcPerProvider: ProviderNeededMap{
			Providers: map[string][]Provider{
				filterChainName: {{
					Name: "ext-proc",
					Extension: &TrafficPolicyGatewayExtensionIR{
						Name:    "ext-proc",
						ExtProc: &envoymatchingv3.ExtensionWithMatcher{},
					},
					FilterStage: filters.AfterStage(filters.FaultStage),
				}},
			},
		},
	}
	// staged behind that ext_proc, which is the placement that pulls the decompressors to Fault
	plugin.handleBuffer(filterChainName, &ir.TypedFilterConfigMap{}, &bufferIR{
		perRoute:    &bufferv3.BufferPerRoute{},
		filterStage: new(filters.AfterStage(filters.FaultStage)),
	})

	httpFilters, err := plugin.HttpFilters(
		ir.HttpFiltersContext{},
		ir.FilterChainCommon{FilterChainName: filterChainName},
	)
	require.NoError(t, err)

	sortedFilters := filters.StagedHttpFilterList(httpFilters)
	sortedFilters.Sort()

	byName := make(map[string]filters.FilterStage[filters.WellKnownFilterStage], len(sortedFilters))
	for _, f := range sortedFilters {
		byName[f.Filter.GetName()] = f.Stage
	}

	decompressor := byName[decompressorFilterNameFor(kgateway.CompressionGzip)]
	for _, name := range []string{bufferFilterName, extProcFilterName("ext-proc")} {
		stage, ok := byName[name]
		require.True(t, ok, "%s missing from %v", name, filterNames(sortedFilters))
		assert.Less(t, filters.FilterStageComparison(decompressor, stage), 0,
			"decompressor stage %v must sort ahead of %s at %v on stage alone", decompressor, name, stage)
	}
}

func filterNames(staged filters.StagedHttpFilterList) []string {
	names := make([]string, 0, len(staged))
	for _, f := range staged {
		names = append(names, f.Filter.GetName())
	}
	return names
}
