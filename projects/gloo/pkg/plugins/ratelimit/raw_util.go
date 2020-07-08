package ratelimit

import (
	"context"
	"time"

	envoyvhostratelimit "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyratelimit "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"

	regexutils "github.com/solo-io/gloo/pkg/utils/regexutils"
	gloorl "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func generateEnvoyConfigForCustomFilter(ref core.ResourceRef, timeout *time.Duration, denyOnFail bool) *envoyratelimit.RateLimit {
	return GenerateEnvoyConfigForFilterWith(ref, CustomDomain, customStage, timeout, denyOnFail)
}

func generateCustomEnvoyConfigForVhost(ctx context.Context, rlactions []*gloorl.RateLimitActions) []*envoyvhostratelimit.RateLimit {
	var ret []*envoyvhostratelimit.RateLimit
	for _, rlaction := range rlactions {
		rl := &envoyvhostratelimit.RateLimit{
			Stage: &wrappers.UInt32Value{Value: customStage},
		}
		rl.Actions = ConvertActions(ctx, rlaction.Actions)
		ret = append(ret, rl)
	}
	return ret
}

func ConvertActions(ctx context.Context, actions []*gloorl.Action) []*envoyvhostratelimit.RateLimit_Action {
	var retActions []*envoyvhostratelimit.RateLimit_Action

	for _, action := range actions {
		retActions = append(retActions, convertAction(ctx, action))
	}
	return retActions
}

func convertAction(ctx context.Context, action *gloorl.Action) *envoyvhostratelimit.RateLimit_Action {
	var retAction envoyvhostratelimit.RateLimit_Action

	switch specificAction := action.ActionSpecifier.(type) {
	case *gloorl.Action_SourceCluster_:
		retAction.ActionSpecifier = &envoyvhostratelimit.RateLimit_Action_SourceCluster_{
			SourceCluster: &envoyvhostratelimit.RateLimit_Action_SourceCluster{},
		}
	case *gloorl.Action_DestinationCluster_:
		retAction.ActionSpecifier = &envoyvhostratelimit.RateLimit_Action_DestinationCluster_{
			DestinationCluster: &envoyvhostratelimit.RateLimit_Action_DestinationCluster{},
		}

	case *gloorl.Action_RequestHeaders_:
		retAction.ActionSpecifier = &envoyvhostratelimit.RateLimit_Action_RequestHeaders_{
			RequestHeaders: &envoyvhostratelimit.RateLimit_Action_RequestHeaders{
				HeaderName:    specificAction.RequestHeaders.GetHeaderName(),
				DescriptorKey: specificAction.RequestHeaders.GetDescriptorKey(),
			},
		}

	case *gloorl.Action_RemoteAddress_:
		retAction.ActionSpecifier = &envoyvhostratelimit.RateLimit_Action_RemoteAddress_{
			RemoteAddress: &envoyvhostratelimit.RateLimit_Action_RemoteAddress{},
		}

	case *gloorl.Action_GenericKey_:
		retAction.ActionSpecifier = &envoyvhostratelimit.RateLimit_Action_GenericKey_{
			GenericKey: &envoyvhostratelimit.RateLimit_Action_GenericKey{
				DescriptorValue: specificAction.GenericKey.GetDescriptorValue(),
			},
		}

	case *gloorl.Action_HeaderValueMatch_:
		retAction.ActionSpecifier = &envoyvhostratelimit.RateLimit_Action_HeaderValueMatch_{
			HeaderValueMatch: &envoyvhostratelimit.RateLimit_Action_HeaderValueMatch{
				ExpectMatch:     gogoutils.BoolGogoToProto(specificAction.HeaderValueMatch.GetExpectMatch()),
				DescriptorValue: specificAction.HeaderValueMatch.GetDescriptorValue(),
				Headers:         convertHeaders(ctx, specificAction.HeaderValueMatch.GetHeaders()),
			},
		}

	}

	return &retAction
}

func convertHeaders(ctx context.Context, headers []*gloorl.HeaderMatcher) []*envoyvhostratelimit.HeaderMatcher {
	var retHeaders []*envoyvhostratelimit.HeaderMatcher
	for _, header := range headers {
		retHeaders = append(retHeaders, convertHeader(ctx, header))
	}
	return retHeaders
}

func convertHeader(ctx context.Context, header *gloorl.HeaderMatcher) *envoyvhostratelimit.HeaderMatcher {
	ret := &envoyvhostratelimit.HeaderMatcher{
		InvertMatch: header.InvertMatch,
		Name:        header.Name,
	}
	switch specificHeaderSpecifier := header.HeaderMatchSpecifier.(type) {
	case *gloorl.HeaderMatcher_ExactMatch:
		ret.HeaderMatchSpecifier = &envoyvhostratelimit.HeaderMatcher_ExactMatch{
			ExactMatch: specificHeaderSpecifier.ExactMatch,
		}
	case *gloorl.HeaderMatcher_RegexMatch:
		ret.HeaderMatchSpecifier = &envoyvhostratelimit.HeaderMatcher_SafeRegexMatch{
			SafeRegexMatch: regexutils.NewRegex(ctx, specificHeaderSpecifier.RegexMatch),
		}
	case *gloorl.HeaderMatcher_RangeMatch:
		ret.HeaderMatchSpecifier = &envoyvhostratelimit.HeaderMatcher_RangeMatch{
			RangeMatch: &envoytype.Int64Range{
				Start: specificHeaderSpecifier.RangeMatch.Start,
				End:   specificHeaderSpecifier.RangeMatch.End,
			},
		}
	case *gloorl.HeaderMatcher_PresentMatch:
		ret.HeaderMatchSpecifier = &envoyvhostratelimit.HeaderMatcher_PresentMatch{
			PresentMatch: specificHeaderSpecifier.PresentMatch,
		}
	case *gloorl.HeaderMatcher_PrefixMatch:
		ret.HeaderMatchSpecifier = &envoyvhostratelimit.HeaderMatcher_PrefixMatch{
			PrefixMatch: specificHeaderSpecifier.PrefixMatch,
		}
	case *gloorl.HeaderMatcher_SuffixMatch:
		ret.HeaderMatchSpecifier = &envoyvhostratelimit.HeaderMatcher_SuffixMatch{
			SuffixMatch: specificHeaderSpecifier.SuffixMatch,
		}
	}
	return ret
}

// TODO: check nil go is annoying.
