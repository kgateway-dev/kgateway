package ratelimit

import (
	"context"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyratelimit "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/regexutils"
	gloorl "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// special value for generic keys that signals the enterprise rate limit server
// to treat those descriptors differently (for the set-style rate-limit API)
const SetDescriptorValue = "solo.setDescriptor.uniqueValue"

func generateEnvoyConfigForCustomFilter(
	ref *core.ResourceRef,
	timeout *duration.Duration,
	denyOnFail bool,
) *envoyratelimit.RateLimit {
	return GenerateEnvoyConfigForFilterWith(ref, CustomDomain, CustomStage, timeout, denyOnFail)
}

func generateCustomEnvoyConfigForVhost(
	ctx context.Context,
	rlactions []*gloorl.RateLimitActions,
) []*envoy_config_route_v3.RateLimit {
	var ret []*envoy_config_route_v3.RateLimit
	for _, rlaction := range rlactions {
		if len(rlaction.Actions) != 0 {
			rl := &envoy_config_route_v3.RateLimit{
				Stage: &wrappers.UInt32Value{Value: CustomStage},
			}
			rl.Actions = ConvertActions(ctx, rlaction.Actions)
			ret = append(ret, rl)
		}
	}
	return ret
}

func ConvertActions(ctx context.Context, actions []*gloorl.Action) []*envoy_config_route_v3.RateLimit_Action {
	var retActions []*envoy_config_route_v3.RateLimit_Action

	var isSetStyle bool
	for _, action := range actions {
		convertedAction := convertAction(ctx, action)
		if convertedAction.GetGenericKey().GetDescriptorValue() == SetDescriptorValue {
			isSetStyle = true
		}
		retActions = append(retActions, convertedAction)
	}

	if isSetStyle {
		for _, action := range retActions {
			hdrs := action.GetRequestHeaders()
			if hdrs != nil {
				hdrs.SkipIfAbsent = true // necessary for the set actions to work; not all request headers may be present
			}
		}
	}

	return retActions
}

func convertAction(ctx context.Context, action *gloorl.Action) *envoy_config_route_v3.RateLimit_Action {
	var retAction envoy_config_route_v3.RateLimit_Action

	switch specificAction := action.ActionSpecifier.(type) {
	case *gloorl.Action_SourceCluster_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_SourceCluster_{
			SourceCluster: &envoy_config_route_v3.RateLimit_Action_SourceCluster{},
		}
	case *gloorl.Action_DestinationCluster_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_DestinationCluster_{
			DestinationCluster: &envoy_config_route_v3.RateLimit_Action_DestinationCluster{},
		}

	case *gloorl.Action_RequestHeaders_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_RequestHeaders_{
			RequestHeaders: &envoy_config_route_v3.RateLimit_Action_RequestHeaders{
				HeaderName:    specificAction.RequestHeaders.GetHeaderName(),
				DescriptorKey: specificAction.RequestHeaders.GetDescriptorKey(),
			},
		}

	case *gloorl.Action_RemoteAddress_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
			RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
		}

	case *gloorl.Action_GenericKey_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_GenericKey_{
			GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
				DescriptorValue: specificAction.GenericKey.GetDescriptorValue(),
			},
		}

	case *gloorl.Action_HeaderValueMatch_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch_{
			HeaderValueMatch: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch{
				ExpectMatch:     specificAction.HeaderValueMatch.GetExpectMatch(),
				DescriptorValue: specificAction.HeaderValueMatch.GetDescriptorValue(),
				Headers:         convertHeaders(ctx, specificAction.HeaderValueMatch.GetHeaders()),
			},
		}

	}

	return &retAction
}

func convertHeaders(ctx context.Context, headers []*gloorl.Action_HeaderValueMatch_HeaderMatcher) []*envoy_config_route_v3.HeaderMatcher {
	var retHeaders []*envoy_config_route_v3.HeaderMatcher
	for _, header := range headers {
		retHeaders = append(retHeaders, convertHeader(ctx, header))
	}
	return retHeaders
}

func convertHeader(ctx context.Context, header *gloorl.Action_HeaderValueMatch_HeaderMatcher) *envoy_config_route_v3.HeaderMatcher {
	ret := &envoy_config_route_v3.HeaderMatcher{
		InvertMatch: header.InvertMatch,
		Name:        header.Name,
	}
	switch specificHeaderSpecifier := header.HeaderMatchSpecifier.(type) {
	case *gloorl.Action_HeaderValueMatch_HeaderMatcher_ExactMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_ExactMatch{
			ExactMatch: specificHeaderSpecifier.ExactMatch,
		}
	case *gloorl.Action_HeaderValueMatch_HeaderMatcher_RegexMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_SafeRegexMatch{
			SafeRegexMatch: regexutils.NewRegex(ctx, specificHeaderSpecifier.RegexMatch),
		}
	case *gloorl.Action_HeaderValueMatch_HeaderMatcher_RangeMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_RangeMatch{
			RangeMatch: &envoy_type_v3.Int64Range{
				Start: specificHeaderSpecifier.RangeMatch.Start,
				End:   specificHeaderSpecifier.RangeMatch.End,
			},
		}
	case *gloorl.Action_HeaderValueMatch_HeaderMatcher_PresentMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_PresentMatch{
			PresentMatch: specificHeaderSpecifier.PresentMatch,
		}
	case *gloorl.Action_HeaderValueMatch_HeaderMatcher_PrefixMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_PrefixMatch{
			PrefixMatch: specificHeaderSpecifier.PrefixMatch,
		}
	case *gloorl.Action_HeaderValueMatch_HeaderMatcher_SuffixMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_SuffixMatch{
			SuffixMatch: specificHeaderSpecifier.SuffixMatch,
		}
	}
	return ret
}

// TODO: check nil go is annoying.
