package krtcollections

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func (h *RoutesIndex) transformGRPCRoute(kctx krt.HandlerContext, i *gwv1.GRPCRoute) *ir.HttpRouteIR {
	src := ir.ObjectSource{
		Group:     gwv1.SchemeGroupVersion.Group,
		Kind:      wellknown.GRPCRouteKind,
		Namespace: i.Namespace,
		Name:      i.Name,
	}

	return &ir.HttpRouteIR{
		ObjectSource:     src,
		SourceObject:     i,
		ParentRefs:       i.Spec.ParentRefs,
		Hostnames:        tostr(i.Spec.Hostnames),
		Rules:            h.transformGRPCRulesToHttp(kctx, src, i.Spec.Rules),
		AttachedPolicies: toAttachedPolicies(h.policies.getTargetingPolicies(kctx, extensionsplug.RouteAttachmentPoint, src, "", i.GetLabels())),
		// IsHTTP2: true
	}
}

func (h *RoutesIndex) transformGRPCRulesToHttp(kctx krt.HandlerContext, src ir.ObjectSource, rules []gwv1.GRPCRouteRule) []ir.HttpRouteRuleIR {
	httpRules := make([]ir.HttpRouteRuleIR, 0, len(rules))
	for _, r := range rules {
		httpMatches := h.convertGRPCMatchesToHTTP(r.Matches)
		httpBackends := h.convertGRPCBackendsToHTTP(kctx, src, r.BackendRefs)

		httpRules = append(httpRules, ir.HttpRouteRuleIR{
			Backends: httpBackends,
			Matches:  httpMatches,
			Name:     emptyIfNil(r.Name),
		})
	}
	return httpRules
}

func (h *RoutesIndex) convertGRPCMatchesToHTTP(matches []gwv1.GRPCRouteMatch) []gwv1.HTTPRouteMatch {
	httpMatches := make([]gwv1.HTTPRouteMatch, 0, len(matches))
	for _, match := range matches {
		httpMatches = append(httpMatches, grpcToHTTPRouteMatch(match))
	}
	return httpMatches
}

func grpcToHTTPRouteMatch(match gwv1.GRPCRouteMatch) gwv1.HTTPRouteMatch {
	path, pathType := buildGRPCPathMatch(match.Method)
	httpHeaders := convertGRPCHeadersToHTTP(match.Headers)

	return gwv1.HTTPRouteMatch{
		Path: &gwv1.HTTPPathMatch{
			Type:  &pathType,
			Value: &path,
		},
		Headers: httpHeaders,
	}
}

func buildGRPCPathMatch(method *gwv1.GRPCMethodMatch) (string, gwv1.PathMatchType) {
	var path string
	var pathType gwv1.PathMatchType

	if method == nil {
		// If no method match, match all paths
		return "/", gwv1.PathMatchPathPrefix
	}

	switch ptr.Deref(method.Type, gwv1.GRPCMethodMatchExact) {
	case gwv1.GRPCMethodMatchRegularExpression:
		pathType = gwv1.PathMatchRegularExpression
		switch {
		case method.Service != nil && method.Method != nil:
			path = fmt.Sprintf("/%s/%s", string(*method.Service), string(*method.Method))
		case method.Service != nil:
			// Match any valid method name within the service
			path = fmt.Sprintf("/%s/.+", string(*method.Service))
		case method.Method != nil:
			// Match any valid service name before the method
			path = fmt.Sprintf("/.+/%s", string(*method.Method))
		}
	default: // gwv1.GRPCMethodMatchExact
		switch {
		case method.Service != nil && method.Method != nil:
			path = fmt.Sprintf("/%s/%s", string(*method.Service), string(*method.Method))
			pathType = gwv1.PathMatchExact
		case method.Service != nil:
			// Exact service match maps to prefix /service
			path = fmt.Sprintf("/%s", string(*method.Service))
			pathType = gwv1.PathMatchPathPrefix
		case method.Method != nil:
			// Exact method without service isn't directly mappable, use regex
			path = fmt.Sprintf("/.+/%s", string(*method.Method))
			pathType = gwv1.PathMatchRegularExpression
		}
	}

	return path, pathType
}

func convertGRPCHeadersToHTTP(headers []gwv1.GRPCHeaderMatch) []gwv1.HTTPHeaderMatch {
	httpHeaders := make([]gwv1.HTTPHeaderMatch, 0, len(headers))
	for _, h := range headers {
		// Type is passed directly to the HTTPHeaderMatch
		httpHeaders = append(httpHeaders, gwv1.HTTPHeaderMatch{
			Name:  gwv1.HTTPHeaderName(h.Name),
			Value: h.Value,
			Type:  (*gwv1.HeaderMatchType)(h.Type),
		})
	}
	return httpHeaders
}

func (h *RoutesIndex) convertGRPCBackendsToHTTP(kctx krt.HandlerContext, src ir.ObjectSource, backendRefs []gwv1.GRPCBackendRef) []ir.HttpBackendOrDelegate {
	httpBackends := make([]ir.HttpBackendOrDelegate, 0, len(backendRefs))
	for _, ref := range backendRefs {
		backend, err := h.backends.GetBackendFromRef(kctx, src, ref.BackendObjectReference)
		clusterName := "blackhole-cluster"
		if backend != nil {
			clusterName = backend.ClusterName()
		} else if err == nil {
			err = &NotFoundError{NotFoundObj: toFromBackendRef(src.Namespace, ref.BackendObjectReference)}
		}
		httpBackends = append(httpBackends, ir.HttpBackendOrDelegate{
			Backend: &ir.BackendRefIR{
				BackendObject: backend,
				ClusterName:   clusterName,
				Weight:        weight(ref.Weight),
				Err:           err,
			},
		})
	}
	return httpBackends
}
