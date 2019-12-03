package gogoutils

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type"
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

// used in enterprise code
func ToGlooRouteMatch(routeMatch *envoyroute.RouteMatch) *envoyroute_gloo.RouteMatch {
	if routeMatch == nil {
		return nil
	}
	rm := &envoyroute_gloo.RouteMatch{
		PathSpecifier:   nil, // gets set later in function
		CaseSensitive:   BoolProtoToGogo(routeMatch.GetCaseSensitive()),
		RuntimeFraction: ToGlooRuntimeFractionalPercent(routeMatch.GetRuntimeFraction()),
		Headers:         ToGlooHeaders(routeMatch.GetHeaders()),
		QueryParameters: ToGlooQueryParameterMatchers(routeMatch.GetQueryParameters()),
		Grpc:            ToGlooGrpc(routeMatch.GetGrpc()),
	}
	switch typed := routeMatch.GetPathSpecifier().(type) {
	case *envoyroute.RouteMatch_Prefix:
		rm.PathSpecifier = &envoyroute_gloo.RouteMatch_Prefix{
			Prefix: typed.Prefix,
		}
	case *envoyroute.RouteMatch_Regex:
		rm.PathSpecifier = &envoyroute_gloo.RouteMatch_Regex{
			Regex: typed.Regex,
		}
	case *envoyroute.RouteMatch_Path:
		rm.PathSpecifier = &envoyroute_gloo.RouteMatch_Path{
			Path: typed.Path,
		}
	}
	return rm
}

func ToGlooRuntimeFractionalPercent(fp *envoycore.RuntimeFractionalPercent) *envoycore_sk.RuntimeFractionalPercent {
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

func ToGlooHeaders(headers []*envoyroute.HeaderMatcher) []*envoyroute_gloo.HeaderMatcher {
	if headers == nil {
		return nil
	}
	result := make([]*envoyroute_gloo.HeaderMatcher, len(headers))
	for i, v := range headers {
		result[i] = ToGlooHeader(v)
	}
	return result
}

func ToGlooHeader(header *envoyroute.HeaderMatcher) *envoyroute_gloo.HeaderMatcher {
	if header == nil {
		return nil
	}
	h := &envoyroute_gloo.HeaderMatcher{
		Name:                 header.GetName(),
		HeaderMatchSpecifier: nil, // gets set later in function
		InvertMatch:          header.GetInvertMatch(),
	}
	switch specificHeaderSpecifier := header.HeaderMatchSpecifier.(type) {
	case *envoyroute.HeaderMatcher_ExactMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_ExactMatch{
			ExactMatch: specificHeaderSpecifier.ExactMatch,
		}
	case *envoyroute.HeaderMatcher_RegexMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_RegexMatch{
			RegexMatch: specificHeaderSpecifier.RegexMatch,
		}
	case *envoyroute.HeaderMatcher_RangeMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_RangeMatch{
			RangeMatch: &envoytype_gloo.Int64Range{
				Start: specificHeaderSpecifier.RangeMatch.Start,
				End:   specificHeaderSpecifier.RangeMatch.End,
			},
		}
	case *envoyroute.HeaderMatcher_PresentMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_PresentMatch{
			PresentMatch: specificHeaderSpecifier.PresentMatch,
		}
	case *envoyroute.HeaderMatcher_PrefixMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_PrefixMatch{
			PrefixMatch: specificHeaderSpecifier.PrefixMatch,
		}
	case *envoyroute.HeaderMatcher_SuffixMatch:
		h.HeaderMatchSpecifier = &envoyroute_gloo.HeaderMatcher_SuffixMatch{
			SuffixMatch: specificHeaderSpecifier.SuffixMatch,
		}
	}
	return h
}

func ToGlooQueryParameterMatchers(queryParamMatchers []*envoyroute.QueryParameterMatcher) []*envoyroute_gloo.QueryParameterMatcher {
	if queryParamMatchers == nil {
		return nil
	}
	result := make([]*envoyroute_gloo.QueryParameterMatcher, len(queryParamMatchers))
	for i, v := range queryParamMatchers {
		result[i] = ToGlooQueryParameterMatcher(v)
	}
	return result
}

func ToGlooQueryParameterMatcher(queryParamMatcher *envoyroute.QueryParameterMatcher) *envoyroute_gloo.QueryParameterMatcher {
	if queryParamMatcher == nil {
		return nil
	}
	return &envoyroute_gloo.QueryParameterMatcher{
		Name:  queryParamMatcher.GetName(),
		Value: queryParamMatcher.GetValue(),
		Regex: BoolProtoToGogo(queryParamMatcher.GetRegex()),
	}
}

func ToGlooGrpc(grpc *envoyroute.RouteMatch_GrpcRouteMatchOptions) *envoyroute_gloo.RouteMatch_GrpcRouteMatchOptions {
	if grpc == nil {
		return nil
	}
	return &envoyroute_gloo.RouteMatch_GrpcRouteMatchOptions{
		// envoy currently doesn't support any options :/
		// all the more reason to worry about future regressions with this code ala https://github.com/solo-io/gloo/issues/1793
	}
}
