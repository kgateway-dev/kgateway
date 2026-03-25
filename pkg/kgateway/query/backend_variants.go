package query

import (
	"maps"
	"slices"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// BuildGatewayBackendClientCertificateVariants clones every backend reachable
// from the given Gateway's routes into a Gateway-scoped backend when the
// Gateway supplies a backend client certificate. The returned map is keyed by
// the original backend resource name so callers can rewrite route references to
// the Gateway-scoped clone.
func BuildGatewayBackendClientCertificateVariants(
	routes *RoutesForGwResult,
	gateway *ir.Gateway,
	clientCertificate *ir.GatewayBackendClientCertificateIR,
) map[string]*ir.BackendObjectIR {
	if routes == nil || gateway == nil || clientCertificate == nil {
		return nil
	}

	variants := make(map[string]*ir.BackendObjectIR)
	for _, listenerResult := range routes.listenerResults {
		for _, route := range listenerResult.Routes {
			collectRouteBackends(route, func(backend *ir.BackendObjectIR) {
				if backend == nil {
					return
				}

				resourceName := backend.ResourceName()
				if _, ok := variants[resourceName]; ok {
					return
				}

				clone := backend.CloneForGatewayBackendClientCertificate(gateway.ObjectSource, clientCertificate)
				variants[resourceName] = &clone
			})
		}
	}

	return variants
}

// RewriteRoutesForBackendVariants rewrites backend references in the route tree
// to point at the Gateway-scoped backend clones returned by
// BuildGatewayBackendClientCertificateVariants.
func RewriteRoutesForBackendVariants(
	routes *RoutesForGwResult,
	variants map[string]*ir.BackendObjectIR,
) *RoutesForGwResult {
	if routes == nil || len(variants) == 0 {
		return routes
	}

	rewritten := NewRoutesForGwResult()
	rewritten.RouteErrors = append(rewritten.RouteErrors, routes.RouteErrors...)

	for key, listenerResult := range routes.listenerResults {
		clone := &ListenerResult{
			Error:  listenerResult.Error,
			Routes: make([]*RouteInfo, 0, len(listenerResult.Routes)),
		}
		for _, route := range listenerResult.Routes {
			clone.Routes = append(clone.Routes, cloneRouteInfoWithVariants(route, variants))
		}
		rewritten.listenerResults[key] = clone
	}

	return rewritten
}

func cloneRouteInfoWithVariants(route *RouteInfo, variants map[string]*ir.BackendObjectIR) *RouteInfo {
	if route == nil {
		return nil
	}

	clone := &RouteInfo{
		Object:            cloneRouteWithVariants(route.Object, variants),
		ParentRef:         route.ParentRef,
		ListenerParentRef: route.ListenerParentRef,
		HostnameOverrides: slices.Clone(route.HostnameOverrides),
		Children:          cloneChildrenWithVariants(route.Children, variants),
	}

	return clone
}

func cloneChildrenWithVariants(
	children BackendMap[[]*RouteInfo],
	variants map[string]*ir.BackendObjectIR,
) BackendMap[[]*RouteInfo] {
	clone := NewBackendMap[[]*RouteInfo]()
	for key, value := range children.items {
		clonedRoutes := make([]*RouteInfo, 0, len(value))
		for _, child := range value {
			clonedRoutes = append(clonedRoutes, cloneRouteInfoWithVariants(child, variants))
		}
		clone.items[key] = clonedRoutes
	}
	maps.Copy(clone.errors, children.errors)
	return clone
}

func cloneRouteWithVariants(route ir.Route, variants map[string]*ir.BackendObjectIR) ir.Route {
	switch typed := route.(type) {
	case *ir.HttpRouteIR:
		clone := *typed
		clone.ParentRefs = slices.Clone(typed.ParentRefs)
		clone.Hostnames = slices.Clone(typed.Hostnames)
		clone.Rules = make([]ir.HttpRouteRuleIR, len(typed.Rules))
		for i, rule := range typed.Rules {
			clonedRule := rule
			clonedRule.Backends = make([]ir.HttpBackendOrDelegate, len(rule.Backends))
			for j, backend := range rule.Backends {
				clonedRule.Backends[j] = cloneHttpBackendWithVariants(backend, variants)
			}
			clone.Rules[i] = clonedRule
		}
		return &clone
	case *ir.TcpRouteIR:
		clone := *typed
		clone.ParentRefs = slices.Clone(typed.ParentRefs)
		clone.Backends = cloneBackendRefsWithVariants(typed.Backends, variants)
		return &clone
	case *ir.TlsRouteIR:
		clone := *typed
		clone.ParentRefs = slices.Clone(typed.ParentRefs)
		clone.Hostnames = slices.Clone(typed.Hostnames)
		clone.Backends = cloneBackendRefsWithVariants(typed.Backends, variants)
		return &clone
	default:
		return route
	}
}

func cloneHttpBackendWithVariants(
	backend ir.HttpBackendOrDelegate,
	variants map[string]*ir.BackendObjectIR,
) ir.HttpBackendOrDelegate {
	clone := backend
	if backend.Backend == nil {
		return clone
	}

	backendRef := cloneBackendRefWithVariants(*backend.Backend, variants)
	clone.Backend = &backendRef
	return clone
}

func cloneBackendRefsWithVariants(
	backends []ir.BackendRefIR,
	variants map[string]*ir.BackendObjectIR,
) []ir.BackendRefIR {
	cloned := make([]ir.BackendRefIR, len(backends))
	for i, backend := range backends {
		cloned[i] = cloneBackendRefWithVariants(backend, variants)
	}
	return cloned
}

func cloneBackendRefWithVariants(
	backend ir.BackendRefIR,
	variants map[string]*ir.BackendObjectIR,
) ir.BackendRefIR {
	clone := backend
	if backend.BackendObject == nil {
		return clone
	}

	if variant, ok := variants[backend.BackendObject.ResourceName()]; ok {
		clone.BackendObject = variant
		clone.ClusterName = variant.ClusterName()
	}

	return clone
}

func collectRouteBackends(route *RouteInfo, visit func(*ir.BackendObjectIR)) {
	if route == nil {
		return
	}

	switch typed := route.Object.(type) {
	case *ir.HttpRouteIR:
		for _, rule := range typed.Rules {
			for _, backend := range rule.Backends {
				if backend.Backend != nil {
					visit(backend.Backend.BackendObject)
				}
			}
		}
	case *ir.TcpRouteIR:
		for _, backend := range typed.Backends {
			visit(backend.BackendObject)
		}
	case *ir.TlsRouteIR:
		for _, backend := range typed.Backends {
			visit(backend.BackendObject)
		}
	}

	for _, children := range route.Children.items {
		for _, child := range children {
			collectRouteBackends(child, visit)
		}
	}
}
