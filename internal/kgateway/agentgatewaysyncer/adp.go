package agentgatewaysyncer

import (
	"fmt"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func createADPMethodMatch(match gwv1.HTTPRouteMatch) (*api.MethodMatch, *reporter.RouteCondition) {
	if match.Method == nil {
		return nil, nil
	}
	return &api.MethodMatch{
		Exact: string(*match.Method),
	}, nil
}

func createADPQueryMatch(match gwv1.HTTPRouteMatch) ([]*api.QueryMatch, *reporter.RouteCondition) {
	res := []*api.QueryMatch{}
	for _, header := range match.QueryParams {
		tp := gwv1.QueryParamMatchExact
		if header.Type != nil {
			tp = *header.Type
		}
		switch tp {
		case gwv1.QueryParamMatchExact:
			res = append(res, &api.QueryMatch{
				Name:  string(header.Name),
				Value: &api.QueryMatch_Exact{Exact: header.Value},
			})
		case gwv1.QueryParamMatchRegularExpression:
			res = append(res, &api.QueryMatch{
				Name:  string(header.Name),
				Value: &api.QueryMatch_Regex{Regex: header.Value},
			})
		default:
			// Should never happen, unless a new field is added
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonUnsupportedValue,
				Message: fmt.Sprintf("unknown type: %q is not supported QueryMatch type", tp)}
		}
	}
	if len(res) == 0 {
		return nil, nil
	}
	return res, nil
}

func createADPPathMatch(match gwv1.HTTPRouteMatch) (*api.PathMatch, *reporter.RouteCondition) {
	tp := gwv1.PathMatchPathPrefix
	if match.Path == nil {
		return nil, nil
	}
	if match.Path.Type != nil {
		tp = *match.Path.Type
	}
	dest := "/"
	if match.Path.Value != nil {
		dest = *match.Path.Value
	}
	switch tp {
	case gwv1.PathMatchPathPrefix:
		// "When specified, a trailing `/` is ignored."
		if dest != "/" {
			dest = strings.TrimSuffix(dest, "/")
		}
		return &api.PathMatch{Kind: &api.PathMatch_PathPrefix{
			PathPrefix: dest,
		}}, nil
	case gwv1.PathMatchExact:
		return &api.PathMatch{Kind: &api.PathMatch_Exact{
			Exact: dest,
		}}, nil
	case gwv1.PathMatchRegularExpression:
		return &api.PathMatch{Kind: &api.PathMatch_Regex{
			Regex: dest,
		}}, nil
	default:
		// Should never happen, unless a new field is added
		return nil, &reporter.RouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonUnsupportedValue,
			Message: fmt.Sprintf("unknown type: %q is not supported Path match type", tp)}
	}
}

func createADPHeadersMatch(match gwv1.HTTPRouteMatch) ([]*api.HeaderMatch, *reporter.RouteCondition) {
	var res []*api.HeaderMatch
	for _, header := range match.Headers {
		tp := gwv1.HeaderMatchExact
		if header.Type != nil {
			tp = *header.Type
		}
		switch tp {
		case gwv1.HeaderMatchExact:
			res = append(res, &api.HeaderMatch{
				Name:  string(header.Name),
				Value: &api.HeaderMatch_Exact{Exact: header.Value},
			})
		case gwv1.HeaderMatchRegularExpression:
			res = append(res, &api.HeaderMatch{
				Name:  string(header.Name),
				Value: &api.HeaderMatch_Regex{Regex: header.Value},
			})
		default:
			// Should never happen, unless a new field is added
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonUnsupportedValue,
				Message: fmt.Sprintf("unknown type: %q is not supported HeaderMatch type", tp)}
		}
	}

	if len(res) == 0 {
		return nil, nil
	}
	return res, nil
}

func createADPHeadersFilter(filter *gwv1.HTTPHeaderFilter) *api.RouteFilter {
	if filter == nil {
		return nil
	}
	return &api.RouteFilter{
		Kind: &api.RouteFilter_RequestHeaderModifier{
			RequestHeaderModifier: &api.HeaderModifier{
				Add:    headerListToADP(filter.Add),
				Set:    headerListToADP(filter.Set),
				Remove: filter.Remove,
			},
		},
	}
}

func createADPResponseHeadersFilter(filter *gwv1.HTTPHeaderFilter) *api.RouteFilter {
	if filter == nil {
		return nil
	}
	return &api.RouteFilter{
		Kind: &api.RouteFilter_ResponseHeaderModifier{
			ResponseHeaderModifier: &api.HeaderModifier{
				Add:    headerListToADP(filter.Add),
				Set:    headerListToADP(filter.Set),
				Remove: filter.Remove,
			},
		},
	}
}

func createADPRewriteFilter(filter *gwv1.HTTPURLRewriteFilter) *api.RouteFilter {
	if filter == nil {
		return nil
	}

	var hostname string
	if filter.Hostname != nil {
		hostname = string(*filter.Hostname)
	}
	ff := &api.UrlRewrite{
		Host: hostname,
	}
	if filter.Path != nil {
		switch filter.Path.Type {
		case gwv1.PrefixMatchHTTPPathModifier:
			ff.Path = &api.UrlRewrite_Prefix{Prefix: strings.TrimSuffix(*filter.Path.ReplacePrefixMatch, "/")}
		case gwv1.FullPathHTTPPathModifier:
			ff.Path = &api.UrlRewrite_Full{Full: strings.TrimSuffix(*filter.Path.ReplaceFullPath, "/")}
		}
	}
	return &api.RouteFilter{
		Kind: &api.RouteFilter_UrlRewrite{
			UrlRewrite: ff,
		},
	}
}

func createADPMirrorFilter(
	ctx RouteContext,
	filter *gwv1.HTTPRequestMirrorFilter,
	ns string,
	k schema.GroupVersionKind,
) (*api.RouteFilter, *reporter.RouteCondition) {
	if filter == nil {
		return nil, nil
	}
	var weightOne int32 = 1
	dst, err := buildADPDestination(ctx, gwv1.HTTPBackendRef{
		BackendRef: gwv1.BackendRef{
			BackendObjectReference: filter.BackendRef,
			Weight:                 &weightOne,
		},
	}, ns, k, ctx.Backends)
	if err != nil {
		return nil, err
	}
	var percent float64
	if f := filter.Fraction; f != nil {
		denominator := float64(100)
		if f.Denominator != nil {
			denominator = float64(*f.Denominator)
		}
		percent = (100 * float64(f.Numerator)) / denominator
	} else if p := filter.Percent; p != nil {
		percent = float64(*p)
	} else {
		percent = 100
	}
	if percent == 0 {
		return nil, nil
	}
	rm := &api.RequestMirror{
		Percentage: percent,
		Backend:    dst.GetBackend(),
	}
	return &api.RouteFilter{Kind: &api.RouteFilter_RequestMirror{RequestMirror: rm}}, nil
}

func createADPRedirectFilter(filter *gwv1.HTTPRequestRedirectFilter) *api.RouteFilter {
	if filter == nil {
		return nil
	}
	var scheme, host string
	var port, statusCode uint32
	if filter.Scheme != nil {
		scheme = *filter.Scheme
	}
	if filter.Hostname != nil {
		host = string(*filter.Hostname)
	}
	if filter.Port != nil {
		port = uint32(*filter.Port)
	}
	if filter.StatusCode != nil {
		statusCode = uint32(*filter.StatusCode)
	}

	ff := &api.RequestRedirect{
		Scheme: scheme,
		Host:   host,
		Port:   port,
		Status: statusCode,
	}
	if filter.Path != nil {
		switch filter.Path.Type {
		case gwv1.PrefixMatchHTTPPathModifier:
			ff.Path = &api.RequestRedirect_Prefix{Prefix: strings.TrimSuffix(*filter.Path.ReplacePrefixMatch, "/")}
		case gwv1.FullPathHTTPPathModifier:
			ff.Path = &api.RequestRedirect_Full{Full: strings.TrimSuffix(*filter.Path.ReplaceFullPath, "/")}
		}
	}
	return &api.RouteFilter{
		Kind: &api.RouteFilter_RequestRedirect{
			RequestRedirect: ff,
		},
	}
}

func headerListToADP(hl []gwv1.HTTPHeader) []*api.Header {
	return slices.Map(hl, func(hl gwv1.HTTPHeader) *api.Header {
		return &api.Header{
			Name:  string(hl.Name),
			Value: hl.Value,
		}
	})
}

// GRPC-specific ADP conversion functions

func createADPGRPCHeadersMatch(match gwv1.GRPCRouteMatch) ([]*api.HeaderMatch, *reporter.RouteCondition) {
	var res []*api.HeaderMatch
	for _, header := range match.Headers {
		tp := gwv1.GRPCHeaderMatchExact
		if header.Type != nil {
			tp = *header.Type
		}
		switch tp {
		case gwv1.GRPCHeaderMatchExact:
			res = append(res, &api.HeaderMatch{
				Name:  string(header.Name),
				Value: &api.HeaderMatch_Exact{Exact: header.Value},
			})
		case gwv1.GRPCHeaderMatchRegularExpression:
			res = append(res, &api.HeaderMatch{
				Name:  string(header.Name),
				Value: &api.HeaderMatch_Regex{Regex: header.Value},
			})
		default:
			// Should never happen, unless a new field is added
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonUnsupportedValue,
				Message: fmt.Sprintf("unknown type: %q is not supported HeaderMatch type", tp)}
		}
	}

	if len(res) == 0 {
		return nil, nil
	}
	return res, nil
}

func buildADPGRPCFilters(
	ctx RouteContext,
	ns string,
	inputFilters []gwv1.GRPCRouteFilter,
) ([]*api.RouteFilter, *reporter.RouteCondition) {
	var filters []*api.RouteFilter
	var mirrorBackendErr *reporter.RouteCondition
	for _, filter := range inputFilters {
		switch filter.Type {
		case gwv1.GRPCRouteFilterRequestHeaderModifier:
			h := createADPHeadersFilter(filter.RequestHeaderModifier)
			if h == nil {
				continue
			}
			filters = append(filters, h)
		case gwv1.GRPCRouteFilterResponseHeaderModifier:
			h := createADPResponseHeadersFilter(filter.ResponseHeaderModifier)
			if h == nil {
				continue
			}
			filters = append(filters, h)
		case gwv1.GRPCRouteFilterRequestMirror:
			h, err := createADPMirrorFilter(ctx, filter.RequestMirror, ns, schema.GroupVersionKind{
				Group:   "gateway.networking.k8s.io",
				Version: "v1",
				Kind:    "GRPCRoute",
			})
			if err != nil {
				mirrorBackendErr = err
			} else {
				filters = append(filters, h)
			}
		default:
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonIncompatibleFilters,
				Message: fmt.Sprintf("unsupported filter type %q", filter.Type),
			}
		}
	}
	return filters, mirrorBackendErr
}

func buildADPGRPCDestination(
	ctx RouteContext,
	forwardTo []gwv1.GRPCBackendRef,
	ns string,
) ([]*api.RouteBackend, *reporter.RouteCondition, *reporter.RouteCondition) {
	if forwardTo == nil {
		return nil, nil, nil
	}

	var invalidBackendErr *reporter.RouteCondition
	var res []*api.RouteBackend
	for _, fwd := range forwardTo {
		dst, err := buildADPDestination(ctx, gwv1.HTTPBackendRef{
			BackendRef: fwd.BackendRef,
			Filters:    nil, // GRPC filters are handled separately
		}, ns, schema.GroupVersionKind{
			Group:   "gateway.networking.k8s.io",
			Version: "v1",
			Kind:    "GRPCRoute",
		}, ctx.Backends)
		if err != nil {
			logger.Error("error building agent gateway destination", "error", err)
			if isInvalidBackend(err) {
				invalidBackendErr = err
				// keep going, we will gracefully drop invalid backends
			} else {
				return nil, nil, err
			}
		}
		if dst != nil {
			filters, err := buildADPGRPCFilters(ctx, ns, fwd.Filters)
			if err != nil {
				return nil, nil, err
			}
			dst.Filters = filters
		}
		res = append(res, dst)
	}
	return res, invalidBackendErr, nil
}
