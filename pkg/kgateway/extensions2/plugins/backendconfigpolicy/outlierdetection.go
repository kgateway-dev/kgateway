package backendconfigpolicy

import (
	"strconv"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_common_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func translateOutlierDetection(od *kgateway.OutlierDetection) *envoyclusterv3.OutlierDetection {
	if od == nil {
		return nil
	}

	outlierDetection := &envoyclusterv3.OutlierDetection{}

	if od.Consecutive5xx != nil {
		outlierDetection.Consecutive_5Xx = &wrapperspb.UInt32Value{Value: uint32(*od.Consecutive5xx)} // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if od.EnforcingConsecutive5xx != nil {
		outlierDetection.EnforcingConsecutive_5Xx = &wrapperspb.UInt32Value{Value: uint32(*od.EnforcingConsecutive5xx)} // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if od.ConsecutiveGatewayFailure != nil {
		outlierDetection.ConsecutiveGatewayFailure = &wrapperspb.UInt32Value{Value: uint32(*od.ConsecutiveGatewayFailure)} // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		// Envoy defaults gateway failure enforcement to 0%, so enforce fully when enabled.
		outlierDetection.EnforcingConsecutiveGatewayFailure = &wrapperspb.UInt32Value{Value: 100}
	}
	if od.SplitExternalLocalOriginErrors != nil {
		outlierDetection.SplitExternalLocalOriginErrors = *od.SplitExternalLocalOriginErrors
	}
	if od.ConsecutiveLocalOriginFailure != nil {
		outlierDetection.ConsecutiveLocalOriginFailure = &wrapperspb.UInt32Value{Value: uint32(*od.ConsecutiveLocalOriginFailure)} // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if od.EnforcingConsecutiveLocalOriginFailure != nil {
		outlierDetection.EnforcingConsecutiveLocalOriginFailure = &wrapperspb.UInt32Value{Value: uint32(*od.EnforcingConsecutiveLocalOriginFailure)} // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if od.Interval != nil {
		outlierDetection.Interval = durationpb.New(od.Interval.Duration)
	}
	if od.BaseEjectionTime != nil {
		outlierDetection.BaseEjectionTime = durationpb.New(od.BaseEjectionTime.Duration)
	}
	if od.MaxEjectionPercent != nil {
		outlierDetection.MaxEjectionPercent = &wrapperspb.UInt32Value{Value: uint32(*od.MaxEjectionPercent)} // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	return outlierDetection
}

func translateOutlierDetectionErrorMatcher(statusCodes []kgateway.StatusCodeOrRange) *envoy_config_common_matcher_v3.MatchPredicate {
	if len(statusCodes) == 0 {
		return nil
	}

	rules := make([]*envoy_config_common_matcher_v3.MatchPredicate, 0, len(statusCodes))
	for _, sc := range statusCodes {
		var headerMatcher *envoyroutev3.HeaderMatcher
		if sc.Code != nil {
			headerMatcher = &envoyroutev3.HeaderMatcher{
				Name: ":status",
				HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_ExactMatch{
					ExactMatch: strconv.Itoa(int(*sc.Code)),
				},
			}
		} else if sc.Range != nil {
			headerMatcher = &envoyroutev3.HeaderMatcher{
				Name: ":status",
				HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_RangeMatch{
					RangeMatch: &envoytypev3.Int64Range{
						Start: int64(sc.Range.Start),
						End:   int64(sc.Range.End + 1), // Envoy Int64Range is [start, end), so +1 makes it inclusive of end
					},
				},
			}
		} else {
			continue
		}

		rules = append(rules, &envoy_config_common_matcher_v3.MatchPredicate{
			Rule: &envoy_config_common_matcher_v3.MatchPredicate_HttpResponseHeadersMatch{
				HttpResponseHeadersMatch: &envoy_config_common_matcher_v3.HttpHeadersMatch{
					Headers: []*envoyroutev3.HeaderMatcher{headerMatcher},
				},
			},
		})
	}

	if len(rules) == 0 {
		return nil
	}
	if len(rules) == 1 {
		return rules[0]
	}
	return &envoy_config_common_matcher_v3.MatchPredicate{
		Rule: &envoy_config_common_matcher_v3.MatchPredicate_OrMatch{
			OrMatch: &envoy_config_common_matcher_v3.MatchPredicate_MatchSet{
				Rules: rules,
			},
		},
	}
}

func translateOutlierDetectionHttpOptions(od *kgateway.OutlierDetection) *envoy_upstreams_v3.HttpProtocolOptions_OutlierDetection {
	if od == nil || len(od.ErrorStatusCodes) == 0 {
		return nil
	}

	errorMatcher := translateOutlierDetectionErrorMatcher(od.ErrorStatusCodes)
	if errorMatcher == nil {
		return nil
	}

	return &envoy_upstreams_v3.HttpProtocolOptions_OutlierDetection{
		ErrorMatcher: errorMatcher,
	}
}
