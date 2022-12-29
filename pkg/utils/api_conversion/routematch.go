package api_conversion

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	envoyroute_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/route"
	envoytype_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	envoytype_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/type"
)

// Converts between Envoy and Gloo/solokit versions of envoy protos
// This is required because go-control-plane dropped gogoproto in favor of goproto
// in v0.9.0, but solokit depends on gogoproto (and the generated deep equals it creates).
//
// we should work to remove that assumption from solokit and delete this code:
// https://github.com/solo-io/gloo/issues/1793

// todo consider movinng this to solo-projects
// used in enterprise code
func ToGlooRouteMatch(routeMatch *envoy_config_route_v3.RouteMatch) *envoyroute_gloo.RouteMatch {
	if routeMatch == nil {
		return nil
	}
	rm := &envoyroute_gloo.RouteMatch{
		PathSpecifier:   nil, // gets set later in function
		CaseSensitive:   routeMatch.GetCaseSensitive(),
		RuntimeFraction: ToGlooRuntimeFractionalPercent(routeMatch.GetRuntimeFraction()),
		Headers:         ToGlooHeaders(routeMatch.GetHeaders()),
		QueryParameters: ToGlooQueryParameterMatchers(routeMatch.GetQueryParameters()),
		Grpc:            ToGlooGrpc(routeMatch.GetGrpc()),
	}
	switch typed := routeMatch.GetPathSpecifier().(type) {
	case *envoy_config_route_v3.RouteMatch_Prefix:
		rm.PathSpecifier = &envoyroute_gloo.RouteMatch_Prefix{
			Prefix: typed.Prefix,
		}
	case *envoy_config_route_v3.RouteMatch_SafeRegex:
		rm.PathSpecifier = &envoyroute_gloo.RouteMatch_Regex{
			Regex: typed.SafeRegex.GetRegex(),
		}
	case *envoy_config_route_v3.RouteMatch_Path:
		rm.PathSpecifier = &envoyroute_gloo.RouteMatch_Path{
			Path: typed.Path,
		}
	}
	return rm
}

func ToGlooRuntimeFractionalPercent(fp *envoy_config_core_v3.RuntimeFractionalPercent) *envoycore_sk.RuntimeFractionalPercent {
	if fp == nil {
		return nil
	}
	return &envoycore_sk.RuntimeFractionalPercent{
		DefaultValue: ToGlooFractionalPercent(fp.GetDefaultValue()),
		RuntimeKey:   fp.GetRuntimeKey(),
	}
}

func ToGlooFractionalPercent(fp *envoytype.FractionalPercent) *envoytype_sk.FractionalPercent {
	if fp == nil {
		return nil
	}
	glooFp := &envoytype_sk.FractionalPercent{
		Numerator:   fp.GetNumerator(),
		Denominator: envoytype_sk.FractionalPercent_HUNDRED, // gets set later in function
	}
	switch str := fp.GetDenominator().String(); str {
	case envoytype.FractionalPercent_DenominatorType_name[int32(envoytype.FractionalPercent_HUNDRED)]:
		glooFp.Denominator = envoytype_sk.FractionalPercent_HUNDRED
	case envoytype.FractionalPercent_DenominatorType_name[int32(envoytype.FractionalPercent_TEN_THOUSAND)]:
		glooFp.Denominator = envoytype_sk.FractionalPercent_TEN_THOUSAND
	case envoytype.FractionalPercent_DenominatorType_name[int32(envoytype.FractionalPercent_MILLION)]:
		glooFp.Denominator = envoytype_sk.FractionalPercent_MILLION
	}
	return glooFp
}

func ToGlooHeaders(headers []*envoy_config_route_v3.HeaderMatcher) []*envoyroute_gloo.HeaderMatcher {
	if headers == nil {
		return nil
	}
	result := make([]*envoyroute_gloo.HeaderMatcher, len(headers))
	for i, v := range headers {
		result[i] = ToGlooHeader(v)
	}
	return result
}

func ToGlooHeader(header *envoy_config_route_v3.HeaderMatcher) *envoyroute_gloo.HeaderMatcher {
	if header == nil {
		return nil
	}
	h := &envoyroute_gloo.HeaderMatcher{
		Name:                 header.GetName(),
		HeaderMatchSpecifier: nil, // gets set later in function
		InvertMatch:          header.GetInvertMatch(),
	}
	switch specificHeaderSpecifier := header.GetHeaderMatchSpecifier().(type) {
	case *envoy_config_route_v3.HeaderMatcher_ExactMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_ExactMatch{
			ExactMatch: specificHeaderSpecifier.ExactMatch,
		}
	case *envoy_config_route_v3.HeaderMatcher_SafeRegexMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_RegexMatch{
			RegexMatch: specificHeaderSpecifier.SafeRegexMatch.GetRegex(),
		}
	case *envoy_config_route_v3.HeaderMatcher_RangeMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_RangeMatch{
			RangeMatch: &envoytype_gloo.Int64Range{
				Start: specificHeaderSpecifier.RangeMatch.GetStart(),
				End:   specificHeaderSpecifier.RangeMatch.GetEnd(),
			},
		}
	case *envoy_config_route_v3.HeaderMatcher_PresentMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_PresentMatch{
			PresentMatch: specificHeaderSpecifier.PresentMatch,
		}
	case *envoy_config_route_v3.HeaderMatcher_PrefixMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_PrefixMatch{
			PrefixMatch: specificHeaderSpecifier.PrefixMatch,
		}
	case *envoy_config_route_v3.HeaderMatcher_SuffixMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_SuffixMatch{
			SuffixMatch: specificHeaderSpecifier.SuffixMatch,
		}
	}
	return h
}

func ToGlooQueryParameterMatchers(queryParamMatchers []*envoy_config_route_v3.QueryParameterMatcher) []*envoyroute_gloo.QueryParameterMatcher {
	if queryParamMatchers == nil {
		return nil
	}
	result := make([]*envoyroute_gloo.QueryParameterMatcher, len(queryParamMatchers))
	for i, v := range queryParamMatchers {
		result[i] = ToGlooQueryParameterMatcher(v)
	}
	return result
}

func ToGlooQueryParameterMatcher(queryParamMatcher *envoy_config_route_v3.QueryParameterMatcher) *envoyroute_gloo.QueryParameterMatcher {
	if queryParamMatcher == nil {
		return nil
	}
	value := ""
	regex := false
	switch {
	case queryParamMatcher.GetPresentMatch():
	case queryParamMatcher.GetStringMatch().GetExact() != "":
		value = queryParamMatcher.GetStringMatch().GetExact()
	case queryParamMatcher.GetStringMatch().GetSafeRegex() != nil:
		value = queryParamMatcher.GetStringMatch().GetSafeRegex().GetRegex()
		regex = true
	}

	qpm := &envoyroute_gloo.QueryParameterMatcher{
		Name:  queryParamMatcher.GetName(),
		Value: value,
	}
	if regex {
		qpm.Regex = &wrappers.BoolValue{
			Value: true,
		}
	}
	return qpm
}

func ToGlooGrpc(grpc *envoy_config_route_v3.RouteMatch_GrpcRouteMatchOptions) *envoyroute_gloo.RouteMatch_GrpcRouteMatchOptions {
	if grpc == nil {
		return nil
	}
	return &envoyroute_gloo.RouteMatch_GrpcRouteMatchOptions{
		// envoy currently doesn't support any options :/
		// all the more reason to worry about future regressions with this code ala https://github.com/solo-io/gloo/issues/1793
	}
}
