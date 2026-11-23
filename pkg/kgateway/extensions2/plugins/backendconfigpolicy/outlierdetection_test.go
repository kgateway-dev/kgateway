package backendconfigpolicy

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_common_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestTranslateOutlierDetection(t *testing.T) {
	tests := []struct {
		name     string
		config   *kgateway.OutlierDetection
		expected *envoyclusterv3.OutlierDetection
	}{
		{
			name:     "nil outlier detection",
			config:   nil,
			expected: nil,
		},
		{
			name:     "minimalist outlier detection config",
			config:   &kgateway.OutlierDetection{},
			expected: &envoyclusterv3.OutlierDetection{},
		},
		{
			name: "partial outlier detection config",
			config: &kgateway.OutlierDetection{
				Interval: &metav1.Duration{Duration: 11 * time.Second},
			},
			expected: &envoyclusterv3.OutlierDetection{
				Interval: durationpb.New(11 * time.Second),
			},
		},
		{
			name: "enforce local origin failures and disable external 5xx ejections",
			config: &kgateway.OutlierDetection{
				SplitExternalLocalOriginErrors:         new(true),
				ConsecutiveLocalOriginFailure:          new(int32(10)),
				EnforcingConsecutiveLocalOriginFailure: new(int32(100)),
				EnforcingConsecutive5xx:                new(int32(0)),
			},
			expected: &envoyclusterv3.OutlierDetection{
				SplitExternalLocalOriginErrors:         true,
				ConsecutiveLocalOriginFailure:          &wrapperspb.UInt32Value{Value: 10},
				EnforcingConsecutiveLocalOriginFailure: &wrapperspb.UInt32Value{Value: 100},
				EnforcingConsecutive_5Xx:               &wrapperspb.UInt32Value{Value: 0},
			},
		},
		{
			name: "consecutive gateway failures are fully enforced",
			config: &kgateway.OutlierDetection{
				ConsecutiveGatewayFailure: new(int32(3)),
			},
			expected: &envoyclusterv3.OutlierDetection{
				ConsecutiveGatewayFailure:          &wrapperspb.UInt32Value{Value: 3},
				EnforcingConsecutiveGatewayFailure: &wrapperspb.UInt32Value{Value: 100},
			},
		},
		{
			name: "full outlier detection config",
			config: &kgateway.OutlierDetection{
				Consecutive5xx:     new(int32(2)),
				Interval:           &metav1.Duration{Duration: 5 * time.Second},
				BaseEjectionTime:   &metav1.Duration{Duration: 7 * time.Minute},
				MaxEjectionPercent: new(int32(99)),
			},
			expected: &envoyclusterv3.OutlierDetection{
				Consecutive_5Xx:    &wrapperspb.UInt32Value{Value: 2},
				Interval:           durationpb.New(5 * time.Second),
				BaseEjectionTime:   durationpb.New(7 * time.Minute),
				MaxEjectionPercent: &wrapperspb.UInt32Value{Value: 99},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := translateOutlierDetection(test.config)
			if !proto.Equal(result, test.expected) {
				t.Errorf("expected %v, got %v", test.expected, result)
			}
		})
	}
}

func TestTranslateOutlierDetectionHttpOptions(t *testing.T) {
	status429 := int32(429)
	status503 := int32(503)

	match429 := &envoy_config_common_matcher_v3.MatchPredicate{
		Rule: &envoy_config_common_matcher_v3.MatchPredicate_HttpResponseHeadersMatch{
			HttpResponseHeadersMatch: &envoy_config_common_matcher_v3.HttpHeadersMatch{
				Headers: []*envoyroutev3.HeaderMatcher{
					{
						Name: ":status",
						HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_ExactMatch{
							ExactMatch: "429",
						},
					},
				},
			},
		},
	}

	match503 := &envoy_config_common_matcher_v3.MatchPredicate{
		Rule: &envoy_config_common_matcher_v3.MatchPredicate_HttpResponseHeadersMatch{
			HttpResponseHeadersMatch: &envoy_config_common_matcher_v3.HttpHeadersMatch{
				Headers: []*envoyroutev3.HeaderMatcher{
					{
						Name: ":status",
						HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_ExactMatch{
							ExactMatch: "503",
						},
					},
				},
			},
		},
	}

	match5xxRange := &envoy_config_common_matcher_v3.MatchPredicate{
		Rule: &envoy_config_common_matcher_v3.MatchPredicate_HttpResponseHeadersMatch{
			HttpResponseHeadersMatch: &envoy_config_common_matcher_v3.HttpHeadersMatch{
				Headers: []*envoyroutev3.HeaderMatcher{
					{
						Name: ":status",
						HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_RangeMatch{
							RangeMatch: &envoytypev3.Int64Range{
								Start: 500,
								End:   600,
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		config   *kgateway.OutlierDetection
		expected *envoy_upstreams_v3.HttpProtocolOptions_OutlierDetection
	}{
		{
			name:     "nil outlier detection",
			config:   nil,
			expected: nil,
		},
		{
			name:     "empty error status codes",
			config:   &kgateway.OutlierDetection{},
			expected: nil,
		},
		{
			name: "single status code",
			config: &kgateway.OutlierDetection{
				ErrorStatusCodes: []kgateway.StatusCodeOrRange{
					{Code: &status429},
				},
			},
			expected: &envoy_upstreams_v3.HttpProtocolOptions_OutlierDetection{
				ErrorMatcher: match429,
			},
		},
		{
			name: "single status range",
			config: &kgateway.OutlierDetection{
				ErrorStatusCodes: []kgateway.StatusCodeOrRange{
					{
						Range: &kgateway.HttpStatusRange{
							Start: 500,
							End:   599,
						},
					},
				},
			},
			expected: &envoy_upstreams_v3.HttpProtocolOptions_OutlierDetection{
				ErrorMatcher: match5xxRange,
			},
		},
		{
			name: "multiple status codes and ranges combined with OrMatch",
			config: &kgateway.OutlierDetection{
				ErrorStatusCodes: []kgateway.StatusCodeOrRange{
					{
						Range: &kgateway.HttpStatusRange{
							Start: 500,
							End:   599,
						},
					},
					{Code: &status429},
					{Code: &status503},
				},
			},
			expected: &envoy_upstreams_v3.HttpProtocolOptions_OutlierDetection{
				ErrorMatcher: &envoy_config_common_matcher_v3.MatchPredicate{
					Rule: &envoy_config_common_matcher_v3.MatchPredicate_OrMatch{
						OrMatch: &envoy_config_common_matcher_v3.MatchPredicate_MatchSet{
							Rules: []*envoy_config_common_matcher_v3.MatchPredicate{
								match5xxRange,
								match429,
								match503,
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := translateOutlierDetectionHttpOptions(test.config)
			if test.expected == nil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.True(t, proto.Equal(result, test.expected), "expected %v, got %v", test.expected, result)
			// Validate Envoy proto validation
			assert.NoError(t, result.ValidateAll())
		})
	}
}

func TestApplyOutlierDetectionHttpOptions(t *testing.T) {
	t.Run("nil options does not modify cluster", func(t *testing.T) {
		cluster := &envoyclusterv3.Cluster{}
		applyOutlierDetectionHttpOptions(nil, cluster)
		assert.Empty(t, cluster.TypedExtensionProtocolOptions)
	})

	t.Run("applies outlier detection to cluster http options", func(t *testing.T) {
		cluster := &envoyclusterv3.Cluster{}
		opts := &envoy_upstreams_v3.HttpProtocolOptions_OutlierDetection{
			ErrorMatcher: &envoy_config_common_matcher_v3.MatchPredicate{
				Rule: &envoy_config_common_matcher_v3.MatchPredicate_AnyMatch{AnyMatch: true},
			},
		}

		applyOutlierDetectionHttpOptions(opts, cluster)

		raw, ok := cluster.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
		require.True(t, ok)
		httpOpts := &envoy_upstreams_v3.HttpProtocolOptions{}
		require.NoError(t, raw.UnmarshalTo(httpOpts))
		assert.True(t, proto.Equal(opts, httpOpts.OutlierDetection))
		assert.NotNil(t, httpOpts.GetUpstreamProtocolOptions(), "must set UpstreamProtocolOptions for Envoy validity")
	})
}
