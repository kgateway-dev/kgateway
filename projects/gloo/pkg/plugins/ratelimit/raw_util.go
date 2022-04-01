package ratelimit

import (
	"context"

	envoy_type_metadata_v3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/regexutils"
	solo_rl "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
)

// special value for generic keys that signals the enterprise rate limit server
// to treat those descriptors differently (for the set-style rate-limit API)
const SetDescriptorValue = "solo.setDescriptor.uniqueValue"

func toEnvoyRateLimits(
	ctx context.Context,
	actions []*solo_rl.RateLimitActions,
	stage uint32,
) []*envoy_config_route_v3.RateLimit {
	var ret []*envoy_config_route_v3.RateLimit
	for _, action := range actions {
		if len(action.GetActions()) != 0 {
			rl := &envoy_config_route_v3.RateLimit{
				Stage: &wrappers.UInt32Value{Value: stage},
			}
			rl.Actions = ConvertActions(ctx, action.GetActions())
			ret = append(ret, rl)
		}
	}
	return ret
}

func ConvertActions(ctx context.Context, actions []*solo_rl.Action) []*envoy_config_route_v3.RateLimit_Action {
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

func convertAction(ctx context.Context, action *solo_rl.Action) *envoy_config_route_v3.RateLimit_Action {
	var retAction envoy_config_route_v3.RateLimit_Action

	switch specificAction := action.GetActionSpecifier().(type) {
	case *solo_rl.Action_SourceCluster_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_SourceCluster_{
			SourceCluster: &envoy_config_route_v3.RateLimit_Action_SourceCluster{},
		}
	case *solo_rl.Action_DestinationCluster_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_DestinationCluster_{
			DestinationCluster: &envoy_config_route_v3.RateLimit_Action_DestinationCluster{},
		}

	case *solo_rl.Action_RequestHeaders_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_RequestHeaders_{
			RequestHeaders: &envoy_config_route_v3.RateLimit_Action_RequestHeaders{
				HeaderName:    specificAction.RequestHeaders.GetHeaderName(),
				DescriptorKey: specificAction.RequestHeaders.GetDescriptorKey(),
			},
		}

	case *solo_rl.Action_RemoteAddress_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
			RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
		}

	case *solo_rl.Action_GenericKey_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_GenericKey_{
			GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
				DescriptorValue: specificAction.GenericKey.GetDescriptorValue(),
			},
		}

	case *solo_rl.Action_HeaderValueMatch_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch_{
			HeaderValueMatch: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch{
				ExpectMatch:     specificAction.HeaderValueMatch.GetExpectMatch(),
				DescriptorValue: specificAction.HeaderValueMatch.GetDescriptorValue(),
				Headers:         convertHeaders(ctx, specificAction.HeaderValueMatch.GetHeaders()),
			},
		}

	case *solo_rl.Action_Metadata:

		var envoyPathSegments []*envoy_type_metadata_v3.MetadataKey_PathSegment
		for _, segment := range specificAction.Metadata.GetMetadataKey().GetPath() {
			envoyPathSegments = append(envoyPathSegments, &envoy_type_metadata_v3.MetadataKey_PathSegment{
				Segment: &envoy_type_metadata_v3.MetadataKey_PathSegment_Key{
					Key: segment.GetKey(),
				},
			})
		}

		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_Metadata{
			Metadata: &envoy_config_route_v3.RateLimit_Action_MetaData{
				DescriptorKey: specificAction.Metadata.GetDescriptorKey(),
				MetadataKey: &envoy_type_metadata_v3.MetadataKey{
					Key:  specificAction.Metadata.GetMetadataKey().GetKey(),
					Path: envoyPathSegments,
				},
				DefaultValue: specificAction.Metadata.GetDefaultValue(),
				Source:       envoy_config_route_v3.RateLimit_Action_MetaData_Source(specificAction.Metadata.GetSource()),
			},
		}
	}

	return &retAction
}

func convertHeaders(ctx context.Context, headers []*solo_rl.Action_HeaderValueMatch_HeaderMatcher) []*envoy_config_route_v3.HeaderMatcher {
	var retHeaders []*envoy_config_route_v3.HeaderMatcher
	for _, header := range headers {
		retHeaders = append(retHeaders, convertHeader(ctx, header))
	}
	return retHeaders
}

func convertHeader(ctx context.Context, header *solo_rl.Action_HeaderValueMatch_HeaderMatcher) *envoy_config_route_v3.HeaderMatcher {
	ret := &envoy_config_route_v3.HeaderMatcher{
		InvertMatch: header.GetInvertMatch(),
		Name:        header.GetName(),
	}
	switch specificHeaderSpecifier := header.GetHeaderMatchSpecifier().(type) {
	case *solo_rl.Action_HeaderValueMatch_HeaderMatcher_ExactMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_ExactMatch{
			ExactMatch: specificHeaderSpecifier.ExactMatch,
		}
	case *solo_rl.Action_HeaderValueMatch_HeaderMatcher_RegexMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_SafeRegexMatch{
			SafeRegexMatch: regexutils.NewRegex(ctx, specificHeaderSpecifier.RegexMatch),
		}
	case *solo_rl.Action_HeaderValueMatch_HeaderMatcher_RangeMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_RangeMatch{
			RangeMatch: &envoy_type_v3.Int64Range{
				Start: specificHeaderSpecifier.RangeMatch.GetStart(),
				End:   specificHeaderSpecifier.RangeMatch.GetEnd(),
			},
		}
	case *solo_rl.Action_HeaderValueMatch_HeaderMatcher_PresentMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_PresentMatch{
			PresentMatch: specificHeaderSpecifier.PresentMatch,
		}
	case *solo_rl.Action_HeaderValueMatch_HeaderMatcher_PrefixMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_PrefixMatch{
			PrefixMatch: specificHeaderSpecifier.PrefixMatch,
		}
	case *solo_rl.Action_HeaderValueMatch_HeaderMatcher_SuffixMatch:
		ret.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_SuffixMatch{
			SuffixMatch: specificHeaderSpecifier.SuffixMatch,
		}
	}
	return ret
}

// TODO: check nil go is annoying.
