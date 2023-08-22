package ratelimit

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"

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

// TODO: this error and handling around it can be removed if we eventually generate CRDs with required values from the ratelimit proto.
// Issue: https://github.com/solo-io/gloo/issues/8580
func missingFieldError(field string) error {
	return eris.Errorf("Missing required field in ratelimit action %s", field)
}

func ToEnvoyRateLimits(
	ctx context.Context,
	actions []*solo_rl.RateLimitActions,
	stage uint32,
) ([]*envoy_config_route_v3.RateLimit, error) {
	var ret []*envoy_config_route_v3.RateLimit
	var allErrors error
	for _, action := range actions {
		if len(action.GetActions()) != 0 {
			rl := &envoy_config_route_v3.RateLimit{
				Stage: &wrappers.UInt32Value{Value: stage},
			}
			var err error
			rl.Actions, err = ConvertActions(ctx, action.GetActions())
			if err != nil {
				allErrors = multierror.Append(allErrors, err)
			}
			ret = append(ret, rl)
		}
	}
	return ret, allErrors
}

// ConvertActions generates Envoy RateLimit_Actions from the solo-apis API. It checks that all required fields are set.
func ConvertActions(ctx context.Context, actions []*solo_rl.Action) ([]*envoy_config_route_v3.RateLimit_Action, error) {
	var retActions []*envoy_config_route_v3.RateLimit_Action

	var isSetStyle bool
	var allErrors error
	for _, action := range actions {
		convertedAction, err := convertAction(ctx, action)
		if err != nil {
			allErrors = multierror.Append(allErrors, err)
			continue
		}
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

	return retActions, allErrors
}

func convertAction(ctx context.Context, action *solo_rl.Action) (*envoy_config_route_v3.RateLimit_Action, error) {
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
		headerName := specificAction.RequestHeaders.GetHeaderName()
		if headerName == "" {
			return nil, missingFieldError("HeaderName")
		}
		descriptorKey := specificAction.RequestHeaders.GetDescriptorKey()
		if descriptorKey == "" {
			return nil, missingFieldError("DescriptorKey")
		}
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_RequestHeaders_{
			RequestHeaders: &envoy_config_route_v3.RateLimit_Action_RequestHeaders{
				HeaderName:    headerName,
				DescriptorKey: descriptorKey,
			},
		}

	case *solo_rl.Action_RemoteAddress_:
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
			RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
		}

	case *solo_rl.Action_GenericKey_:
		descriptorValue := specificAction.GenericKey.GetDescriptorValue()
		if descriptorValue == "" {
			return nil, missingFieldError("DescriptorValue")
		}
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_GenericKey_{
			GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
				DescriptorValue: descriptorValue,
			},
		}

	case *solo_rl.Action_HeaderValueMatch_:
		descriptorValue := specificAction.HeaderValueMatch.GetDescriptorValue()
		if descriptorValue == "" {
			return nil, missingFieldError("DescriptorValue")
		}
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch_{
			HeaderValueMatch: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch{
				ExpectMatch:     specificAction.HeaderValueMatch.GetExpectMatch(),
				DescriptorValue: descriptorValue,
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

		descriptorKey := specificAction.Metadata.GetDescriptorKey()
		if descriptorKey == "" {
			return nil, missingFieldError("DescriptorKey")
		}
		metadataKey := specificAction.Metadata.GetMetadataKey().GetKey()
		if metadataKey == "" {
			return nil, missingFieldError("MetadataKey")
		}
		retAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_Metadata{
			Metadata: &envoy_config_route_v3.RateLimit_Action_MetaData{
				DescriptorKey: descriptorKey,
				MetadataKey: &envoy_type_metadata_v3.MetadataKey{
					Key:  metadataKey,
					Path: envoyPathSegments,
				},
				DefaultValue: specificAction.Metadata.GetDefaultValue(),
				Source:       envoy_config_route_v3.RateLimit_Action_MetaData_Source(specificAction.Metadata.GetSource()),
			},
		}
	default:
		return nil, eris.Errorf("Unknown action type")
	}

	return &retAction, nil
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
