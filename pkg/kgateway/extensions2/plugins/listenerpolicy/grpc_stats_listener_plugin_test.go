package listenerpolicy

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	grpcstatsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_stats/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestConvertGrpcStats(t *testing.T) {
	testCases := []struct {
		name     string
		settings *kgateway.HTTPSettings
		expected *grpcstatsv3.FilterConfig
	}{
		{
			name:     "nil produces no filter config",
			settings: &kgateway.HTTPSettings{},
			expected: nil,
		},
		{
			name: "statsForAllMethods true",
			settings: &kgateway.HTTPSettings{
				GrpcStats: &kgateway.GrpcStats{
					StatsForAllMethods: new(true),
				},
			},
			expected: &grpcstatsv3.FilterConfig{
				PerMethodStatSpecifier: &grpcstatsv3.FilterConfig_StatsForAllMethods{
					StatsForAllMethods: wrapperspb.Bool(true),
				},
			},
		},
		{
			name: "statsForAllMethods false",
			settings: &kgateway.HTTPSettings{
				GrpcStats: &kgateway.GrpcStats{
					StatsForAllMethods: new(false),
				},
			},
			expected: &grpcstatsv3.FilterConfig{
				PerMethodStatSpecifier: &grpcstatsv3.FilterConfig_StatsForAllMethods{
					StatsForAllMethods: wrapperspb.Bool(false),
				},
			},
		},
		{
			name: "enableUpstreamStats with statsForAllMethods",
			settings: &kgateway.HTTPSettings{
				GrpcStats: &kgateway.GrpcStats{
					StatsForAllMethods:  new(true),
					EnableUpstreamStats: new(true),
				},
			},
			expected: &grpcstatsv3.FilterConfig{
				EnableUpstreamStats: true,
				PerMethodStatSpecifier: &grpcstatsv3.FilterConfig_StatsForAllMethods{
					StatsForAllMethods: wrapperspb.Bool(true),
				},
			},
		},
		{
			name: "methodAllowlist grouped by service",
			settings: &kgateway.HTTPSettings{
				GrpcStats: &kgateway.GrpcStats{
					MethodAllowlist: []string{
						"/pkg.Foo/Get",
						"/pkg.Foo/List",
						"/pkg.Bar/Ping",
					},
				},
			},
			expected: &grpcstatsv3.FilterConfig{
				PerMethodStatSpecifier: &grpcstatsv3.FilterConfig_IndividualMethodStatsAllowlist{
					IndividualMethodStatsAllowlist: &envoycorev3.GrpcMethodList{
						Services: []*envoycorev3.GrpcMethodList_Service{
							{Name: "pkg.Foo", MethodNames: []string{"Get", "List"}},
							{Name: "pkg.Bar", MethodNames: []string{"Ping"}},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := convertGrpcStats(tc.settings)
			if tc.expected == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Truef(t, proto.Equal(tc.expected, got), "expected:\n%v\ngot:\n%v", tc.expected, got)
		})
	}
}

// TestGrpcStatsHttpFilter verifies the plugin renders the grpc_stats HTTP filter
// into the HCM chain before the router, and omits it when unset.
func TestGrpcStatsHttpFilter(t *testing.T) {
	const port = uint32(8080)

	t.Run("no grpc_stats filter when unset", func(t *testing.T) {
		p := &listenerPolicyPluginGwPass{
			grpcStats: map[uint32]*grpcstatsv3.FilterConfig{},
		}
		got, err := p.HttpFilters(ir.HttpFiltersContext{ListenerPort: port}, ir.FilterChainCommon{})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("renders grpc_stats filter before router", func(t *testing.T) {
		cfg := &grpcstatsv3.FilterConfig{
			PerMethodStatSpecifier: &grpcstatsv3.FilterConfig_StatsForAllMethods{
				StatsForAllMethods: wrapperspb.Bool(true),
			},
		}
		p := &listenerPolicyPluginGwPass{
			grpcStats: map[uint32]*grpcstatsv3.FilterConfig{port: cfg},
		}
		got, err := p.HttpFilters(ir.HttpFiltersContext{ListenerPort: port}, ir.FilterChainCommon{})
		require.NoError(t, err)
		require.Len(t, got, 1)

		assert.Equal(t, "envoy.filters.http.grpc_stats", got[0].Filter.GetName())
		assert.Equal(t, filters.BeforeStage(filters.RouteStage), got[0].Stage)

		var decoded grpcstatsv3.FilterConfig
		require.NoError(t, got[0].Filter.GetTypedConfig().UnmarshalTo(&decoded))
		assert.True(t, decoded.GetStatsForAllMethods().GetValue())
	})
}
