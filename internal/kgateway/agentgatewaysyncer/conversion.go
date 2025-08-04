package agentgatewaysyncer

import (
	"crypto/tls"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/golang/protobuf/ptypes/duration"
	"istio.io/api/annotation"
	istio "istio.io/api/networking/v1alpha3"
	kubecreds "istio.io/istio/pilot/pkg/credentials/kube"
	"istio.io/istio/pilot/pkg/model"
	creds "istio.io/istio/pilot/pkg/model/credentials"
	"istio.io/istio/pilot/pkg/model/kstatus"
	"istio.io/istio/pilot/pkg/serviceregistry/kube"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/host"
	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/kind"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/sets"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

const (
	gatewayTLSTerminateModeKey = "gateway.agentgateway.io/tls-terminate-mode"
)

func convertHTTPRouteToADP(ctx RouteContext, r gwv1.HTTPRouteRule,
	obj *gwv1.HTTPRoute, pos int, matchPos int,
) (*api.Route, *reporter.RouteCondition) {
	res := &api.Route{
		Key:         obj.Namespace + "." + obj.Name + "." + strconv.Itoa(pos) + "." + strconv.Itoa(matchPos),
		RouteName:   obj.Namespace + "/" + obj.Name,
		ListenerKey: "",
		RuleName:    defaultString(r.Name, ""),
	}

	for _, match := range r.Matches {
		path, err := createADPPathMatch(match)
		if err != nil {
			return nil, err
		}
		headers, err := createADPHeadersMatch(match)
		if err != nil {
			return nil, err
		}
		method, err := createADPMethodMatch(match)
		if err != nil {
			return nil, err
		}
		query, err := createADPQueryMatch(match)
		if err != nil {
			return nil, err
		}
		res.Matches = append(res.GetMatches(), &api.RouteMatch{
			Path:        path,
			Headers:     headers,
			Method:      method,
			QueryParams: query,
		})
	}
	filters, err := buildADPFilters(ctx, obj.Namespace, r.Filters)
	if err != nil {
		return nil, err
	}
	res.Filters = filters

	agentGatewayRouteContext := ir.AgentGatewayRouteContext{
		Rule: &r,
	}

	for _, pass := range ctx.pluginPasses {
		if err := pass.ApplyForRoute(&agentGatewayRouteContext, res); err != nil {
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  "PluginError",
				Message: fmt.Sprintf("failed to apply a plugin: %v", err),
			}
		}
	}

	// Retry: todo
	route, backendErr, err := buildADPHTTPDestination(ctx, r.BackendRefs, obj.Namespace)
	if err != nil {
		return nil, err
	}
	res.Backends = route
	res.Hostnames = slices.Map(obj.Spec.Hostnames, func(e gwv1.Hostname) string {
		return string(e)
	})
	return res, backendErr
}

func convertTCPRouteToADP(ctx RouteContext, r gwv1alpha2.TCPRouteRule,
	obj *gwv1alpha2.TCPRoute, pos int,
) (*api.Route, *reporter.RouteCondition) {
	res := &api.Route{
		Key:         obj.Namespace + "." + obj.Name + "." + strconv.Itoa(pos),
		RouteName:   obj.Namespace + "/" + obj.Name,
		ListenerKey: "",
		RuleName:    defaultString(r.Name, ""),
	}

	res.Matches = []*api.RouteMatch{{
		// TCP doesn't have path, headers, method, or query params
		// This is essentially a catch-all match for TCP traffic
	}}

	// Build TCP destinations
	route, backendErr, err := buildADPTCPDestination(ctx, r.BackendRefs, obj.Namespace)
	if err != nil {
		logger.Error("failed to translate tcp destination", "err", err)
		return nil, err
	}
	res.Backends = route

	return res, backendErr
}

func convertGRPCRouteToADP(ctx RouteContext, r gwv1.GRPCRouteRule,
	obj *gwv1.GRPCRoute, pos int,
) (*api.Route, *reporter.RouteCondition) {
	res := &api.Route{
		Key:         obj.Namespace + "." + obj.Name + "." + strconv.Itoa(pos),
		RouteName:   obj.Namespace + "/" + obj.Name,
		ListenerKey: "",
		RuleName:    defaultString(r.Name, ""),
	}

	// Convert GRPC matches to ADP format
	for _, match := range r.Matches {
		headers, err := createADPGRPCHeadersMatch(match)
		if err != nil {
			logger.Error("failed to translate grpc header match", "err", err, "route_name", obj.Name, "route_ns", obj.Namespace)
			return nil, err
		}
		// For GRPC, we don't have path match in the traditional sense, so we'll derive it from method
		var path *api.PathMatch
		if match.Method != nil {
			// Convert GRPC method to path for routing purposes
			if match.Method.Service != nil && match.Method.Method != nil {
				pathStr := fmt.Sprintf("/%s/%s", *match.Method.Service, *match.Method.Method)
				path = &api.PathMatch{Kind: &api.PathMatch_Exact{Exact: pathStr}}
			} else if match.Method.Service != nil {
				pathStr := fmt.Sprintf("/%s/", *match.Method.Service)
				path = &api.PathMatch{Kind: &api.PathMatch_Exact{Exact: pathStr}}
			} else if match.Method.Method != nil {
				// Convert wildcard to regex: "/*/{method}" becomes "/[^/]+/{method}"
				pathStr := fmt.Sprintf("/[^/]+/%s", *match.Method.Method)
				path = &api.PathMatch{Kind: &api.PathMatch_Regex{Regex: pathStr}}
			}
		}
		res.Matches = append(res.GetMatches(), &api.RouteMatch{
			Path:    path,
			Headers: headers,
			// note: the RouteMatch method field only applies for http methods
		})
	}

	filters, err := buildADPGRPCFilters(ctx, obj.Namespace, r.Filters)
	if err != nil {
		logger.Error("failed to translate grpc filter", "err", err, "route_name", obj.Name, "route_ns", obj.Namespace)
		return nil, err
	}
	res.Filters = filters

	route, backendErr, err := buildADPGRPCDestination(ctx, r.BackendRefs, obj.Namespace)
	if err != nil {
		logger.Error("failed to translate grpc destination", "err", err, "route_name", obj.Name, "route_ns", obj.Namespace)
		return nil, err
	}
	res.Backends = route
	res.Hostnames = slices.Map(obj.Spec.Hostnames, func(e gwv1.Hostname) string {
		return string(e)
	})
	return res, backendErr
}

func convertTLSRouteToADP(ctx RouteContext, r gwv1alpha2.TLSRouteRule,
	obj *gwv1alpha2.TLSRoute, pos int,
) (*api.Route, *reporter.RouteCondition) {
	res := &api.Route{
		Key:         obj.Namespace + "." + obj.Name + "." + strconv.Itoa(pos),
		RouteName:   obj.Namespace + "/" + obj.Name,
		ListenerKey: "",
		RuleName:    defaultString(r.Name, ""),
	}

	// TLS routes match on SNI hostnames, but ADP RouteMatch doesn't have direct SNI support
	// For TLS, we create a basic match that accepts all traffic (SNI matching happens at listener level)
	res.Matches = []*api.RouteMatch{{
		// TLS doesn't have path, headers, method, or query params
		// SNI matching is handled at the listener/gateway level
	}}

	// Build TLS destinations
	route, backendErr, err := buildADPTLSDestination(ctx, r.BackendRefs, obj.Namespace)
	if err != nil {
		logger.Error("failed to translate tls destination", "err", err, "route_name", obj.Name, "route_ns", obj.Namespace)
		return nil, err
	}
	res.Backends = route

	// TLS routes have hostnames in the spec (unlike TCP routes)
	res.Hostnames = slices.Map(obj.Spec.Hostnames, func(e gwv1.Hostname) string {
		return string(e)
	})

	return res, backendErr
}

func buildADPTCPDestination(
	ctx RouteContext,
	forwardTo []gwv1.BackendRef,
	ns string,
) ([]*api.RouteBackend, *reporter.RouteCondition, *reporter.RouteCondition) {
	if forwardTo == nil {
		return nil, nil, nil
	}

	var invalidBackendErr *reporter.RouteCondition
	var res []*api.RouteBackend
	for _, fwd := range forwardTo {
		dst, err := buildADPDestination(ctx, gwv1.HTTPBackendRef{
			BackendRef: fwd,
			Filters:    nil, // TCP routes don't have per-backend filters?
		}, ns, wellknown.TCPRouteGVK, ctx.Backends)
		if err != nil {
			logger.Error("error building agent gateway destination", "error", err)
			if isInvalidBackend(err) {
				invalidBackendErr = err
				// keep going, we will gracefully drop invalid backends
			} else {
				return nil, nil, err
			}
		}
		res = append(res, dst)
	}
	return res, invalidBackendErr, nil
}

func buildADPTLSDestination(
	ctx RouteContext,
	forwardTo []gwv1.BackendRef,
	ns string,
) ([]*api.RouteBackend, *reporter.RouteCondition, *reporter.RouteCondition) {
	if forwardTo == nil {
		return nil, nil, nil
	}

	var invalidBackendErr *reporter.RouteCondition
	var res []*api.RouteBackend
	for _, fwd := range forwardTo {
		dst, err := buildADPDestination(ctx, gwv1.HTTPBackendRef{
			BackendRef: fwd,
			Filters:    nil, // TLS routes don't have per-backend filters
		}, ns, wellknown.TLSRouteGVK, ctx.Backends)
		if err != nil {
			logger.Error("error building agent gateway destination", "error", err)
			if isInvalidBackend(err) {
				invalidBackendErr = err
				// keep going, we will gracefully drop invalid backends
			} else {
				return nil, nil, err
			}
		}
		res = append(res, dst)
	}
	return res, invalidBackendErr, nil
}

func buildADPFilters(
	ctx RouteContext,
	ns string,
	inputFilters []gwv1.HTTPRouteFilter,
) ([]*api.RouteFilter, *reporter.RouteCondition) {
	var filters []*api.RouteFilter
	var mirrorBackendErr *reporter.RouteCondition
	for _, filter := range inputFilters {
		switch filter.Type {
		case gwv1.HTTPRouteFilterRequestHeaderModifier:
			h := createADPHeadersFilter(filter.RequestHeaderModifier)
			if h == nil {
				continue
			}
			filters = append(filters, h)
		case gwv1.HTTPRouteFilterResponseHeaderModifier:
			h := createADPResponseHeadersFilter(filter.ResponseHeaderModifier)
			if h == nil {
				continue
			}
			filters = append(filters, h)
		case gwv1.HTTPRouteFilterRequestRedirect:
			h := createADPRedirectFilter(filter.RequestRedirect)
			if h == nil {
				continue
			}
			filters = append(filters, h)
		case gwv1.HTTPRouteFilterRequestMirror:
			h, err := createADPMirrorFilter(ctx, filter.RequestMirror, ns, wellknown.HTTPRouteGVK)
			if err != nil {
				mirrorBackendErr = err
			} else {
				filters = append(filters, h)
			}
		case gwv1.HTTPRouteFilterURLRewrite:
			h := createADPRewriteFilter(filter.URLRewrite)
			if h == nil {
				continue
			}
			filters = append(filters, h)
		case gwv1.HTTPRouteFilterCORS:
			h := createADPCorsFilter(filter.CORS)
			if h == nil {
				continue
			}
			filters = append(filters, h)
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

func createADPCorsFilter(cors *gwv1.HTTPCORSFilter) *api.RouteFilter {
	if cors == nil {
		return nil
	}
	return &api.RouteFilter{
		Kind: &api.RouteFilter_Cors{Cors: &api.CORS{
			AllowCredentials: bool(cors.AllowCredentials),
			AllowHeaders:     slices.Map(cors.AllowHeaders, func(h gwv1.HTTPHeaderName) string { return string(h) }),
			AllowMethods:     slices.Map(cors.AllowMethods, func(m gwv1.HTTPMethodWithWildcard) string { return string(m) }),
			AllowOrigins:     slices.Map(cors.AllowOrigins, func(o gwv1.AbsoluteURI) string { return string(o) }),
			ExposeHeaders:    slices.Map(cors.ExposeHeaders, func(h gwv1.HTTPHeaderName) string { return string(h) }),
			MaxAge: &duration.Duration{
				Seconds: int64(cors.MaxAge),
			},
		}},
	}
}

func buildADPHTTPDestination(
	ctx RouteContext,
	forwardTo []gwv1.HTTPBackendRef,
	ns string,
) ([]*api.RouteBackend, *reporter.RouteCondition, *reporter.RouteCondition) {
	if forwardTo == nil {
		return nil, nil, nil
	}

	var invalidBackendErr *reporter.RouteCondition
	var res []*api.RouteBackend
	for _, fwd := range forwardTo {
		dst, err := buildADPDestination(ctx, fwd, ns, wellknown.HTTPRouteGVK, ctx.Backends)
		if err != nil {
			logger.Error("erroring building agent gateway destination", "error", err)
			if isInvalidBackend(err) {
				invalidBackendErr = err
				// keep going, we will gracefully drop invalid backends
			} else {
				return nil, nil, err
			}
		}
		if dst != nil {
			filters, err := buildADPFilters(ctx, ns, fwd.Filters)
			if err != nil {
				return nil, nil, err
			}
			dst.Filters = filters
		}
		res = append(res, dst)
	}
	return res, invalidBackendErr, nil
}

func buildADPDestination(
	ctx RouteContext,
	to gwv1.HTTPBackendRef,
	ns string,
	k schema.GroupVersionKind,
	backendCol *krtcollections.BackendIndex,
) (*api.RouteBackend, *reporter.RouteCondition) {
	// check if the reference is allowed
	if toNs := to.Namespace; toNs != nil && string(*toNs) != ns {
		if !ctx.Grants.BackendAllowed(ctx.Krt, k, to.Name, *toNs, ns) {
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionResolvedRefs,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonRefNotPermitted,
				Message: fmt.Sprintf("backendRef %v/%v not accessible to a %s in namespace %q (missing a ReferenceGrant?)", to.Name, *toNs, k.Kind, ns),
			}
		}
	}

	namespace := ns // use default
	if to.Namespace != nil {
		namespace = string(*to.Namespace)
	}
	var invalidBackendErr *reporter.RouteCondition
	var hostname string
	weight := int32(1) // default
	if to.Weight != nil {
		weight = *to.Weight
	}
	rb := &api.RouteBackend{
		Weight: weight,
	}
	var port *gwv1.PortNumber
	ref := normalizeReference(to.Group, to.Kind, wellknown.ServiceGVK)
	switch ref.GroupKind() {
	case wellknown.InferencePoolGVK.GroupKind():
		if strings.Contains(string(to.Name), ".") {
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonUnsupportedValue,
				Message: "service name invalid; the name of the Service must be used, not the hostname."}
		}
		hostname = fmt.Sprintf("%s.%s.inference.%s", to.Name, namespace, ctx.DomainSuffix)
		key := namespace + "/" + string(to.Name)
		svc := ptr.Flatten(krt.FetchOne(ctx.Krt, ctx.InferencePools, krt.FilterKey(key)))
		logger.Debug("found pull pool for service", "svc", svc, "key", key)
		if svc == nil {
			invalidBackendErr = &reporter.RouteCondition{
				Type:    gwv1.RouteConditionResolvedRefs,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonBackendNotFound,
				Message: fmt.Sprintf("backend(%s) not found", hostname)}
		} else {
			rb.Backend = &api.BackendReference{
				Kind: &api.BackendReference_Service{
					Service: namespace + "/" + hostname,
				},
				Port: uint32(svc.Spec.TargetPortNumber),
			}
		}
	case wellknown.ServiceGVK.GroupKind():
		port = to.Port
		if strings.Contains(string(to.Name), ".") {
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonUnsupportedValue,
				Message: "service name invalid; the name of the Service must be used, not the hostname."}
		}
		hostname = fmt.Sprintf("%s.%s.svc.%s", to.Name, namespace, ctx.DomainSuffix)
		key := namespace + "/" + string(to.Name)
		svc := ptr.Flatten(krt.FetchOne(ctx.Krt, ctx.Services, krt.FilterKey(key)))
		if svc == nil {
			invalidBackendErr = &reporter.RouteCondition{
				Type:    gwv1.RouteConditionResolvedRefs,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonBackendNotFound,
				Message: fmt.Sprintf("backend(%s) not found", hostname)}
		}
		// TODO: All kubernetes service types currently require a Port, so we do this for everything; consider making this per-type if we have future types
		// that do not require port.
		if port == nil {
			// "Port is required when the referent is a Kubernetes Service."
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonUnsupportedValue,
				Message: "port is required in backendRef"}
		}
		rb.Backend = &api.BackendReference{
			Kind: &api.BackendReference_Service{
				Service: namespace + "/" + hostname,
			},
			Port: uint32(*port),
		}
	case wellknown.BackendGVK.GroupKind():
		// Create the source ObjectSource representing the route object making the reference
		routeSrc := ir.ObjectSource{
			Group:     k.Group,
			Kind:      k.Kind,
			Namespace: ns,
		}

		// Create the backend reference from the 'to' parameter
		backendRef := gwv1.BackendObjectReference{
			Group:     to.Group,
			Kind:      to.Kind,
			Name:      to.Name,
			Namespace: to.Namespace,
			Port:      to.Port,
		}

		kgwBackend, err := backendCol.GetBackendFromRef(ctx.Krt, routeSrc, backendRef)
		if err != nil {
			logger.Error("failed to get kgateway Backend", "error", err)
			return nil, &reporter.RouteCondition{
				Type:    gwv1.RouteConditionResolvedRefs,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonBackendNotFound,
				Message: fmt.Sprintf("kgateway Backend not found: %v", err),
			}
		}

		logger.Debug("successfully resolved kgateway Backend", "backend", kgwBackend.Name)
		rb.Backend = &api.BackendReference{
			Kind: &api.BackendReference_Backend{
				Backend: kgwBackend.Namespace + "/" + kgwBackend.Name,
			},
		}
	default:
		return nil, &reporter.RouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonInvalidKind,
			Message: fmt.Sprintf("referencing unsupported backendRef: group %q kind %q", ptr.OrEmpty(to.Group), ptr.OrEmpty(to.Kind)),
		}
	}
	return rb, invalidBackendErr
}

func parentMeta(obj controllers.Object, sectionName *gwv1.SectionName) map[string]string {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	name := fmt.Sprintf("%s/%s.%s", kind, obj.GetName(), obj.GetNamespace())
	if sectionName != nil {
		name = fmt.Sprintf("%s/%s/%s.%s", kind, obj.GetName(), *sectionName, obj.GetNamespace())
	}
	return map[string]string{
		constants.InternalParentNames: name,
	}
}

var allowedParentReferences = sets.New(
	wellknown.GatewayGVK,
	wellknown.ServiceGVK,
	wellknown.ServiceEntryGVK,
)

// normalizeReference normalizes group and kind references to a standard GVK format.
// If group or kind are nil/empty, it uses the default GVK's group/kind.
// Empty group is treated as "core" API group.
func normalizeReference(group *gwv1.Group, kind *gwv1.Kind, defaultGVK schema.GroupVersionKind) schema.GroupVersionKind {
	result := defaultGVK

	if kind != nil && *kind != "" {
		result.Kind = string(*kind)
	}

	if group != nil {
		groupStr := string(*group)
		if groupStr == "" {
			// Empty group means "core" API group
			result.Group = ""
		} else {
			result.Group = groupStr
		}
	}

	return result
}

func toInternalParentReference(p gwv1.ParentReference, localNamespace string) (parentKey, error) {
	ref := normalizeReference(p.Group, p.Kind, wellknown.GatewayGVK)
	if !allowedParentReferences.Contains(wellknown.GatewayGVK) {
		return parentKey{}, fmt.Errorf("unsupported parent: %v/%v", p.Group, p.Kind)
	}
	return parentKey{
		Kind: ref,
		Name: string(p.Name),
		// Unset namespace means "same namespace"
		Namespace: defaultString(p.Namespace, localNamespace),
	}, nil
}

func referenceAllowed(
	ctx RouteContext,
	parent *parentInfo,
	routeKind schema.GroupVersionKind,
	parentRef parentReference,
	hostnames []gwv1.Hostname,
	localNamespace string,
) *ParentError {
	if parentRef.Kind == wellknown.ServiceGVK {
		key := parentRef.Namespace + "/" + parentRef.Name
		svc := ptr.Flatten(krt.FetchOne(ctx.Krt, ctx.Services, krt.FilterKey(key)))

		// check that the referenced svc exists
		if svc == nil {
			return &ParentError{
				Reason:  ParentErrorNotAccepted,
				Message: fmt.Sprintf("parent service: %q not found", parentRef.Name),
			}
		}
	} else if parentRef.Kind == wellknown.ServiceEntryGVK {
		// check that the referenced svc entry exists
		key := parentRef.Namespace + "/" + parentRef.Name
		svcEntry := ptr.Flatten(krt.FetchOne(ctx.Krt, ctx.ServiceEntries, krt.FilterKey(key)))
		if svcEntry == nil {
			return &ParentError{
				Reason:  ParentErrorNotAccepted,
				Message: fmt.Sprintf("parent service entry: %q not found", parentRef.Name),
			}
		}
	} else {
		// First, check section and port apply. This must come first
		if parentRef.Port != 0 && parentRef.Port != parent.Port {
			return &ParentError{
				Reason:  ParentErrorNotAccepted,
				Message: fmt.Sprintf("port %v not found", parentRef.Port),
			}
		}
		if len(parentRef.SectionName) > 0 && parentRef.SectionName != parent.SectionName {
			return &ParentError{
				Reason:  ParentErrorNotAccepted,
				Message: fmt.Sprintf("sectionName %q not found", parentRef.SectionName),
			}
		}

		// Next check the hostnames are a match. This is a bi-directional wildcard match. Only one route
		// hostname must match for it to be allowed (but the others will be filtered at runtime)
		// If either is empty its treated as a wildcard which always matches

		if len(hostnames) == 0 {
			hostnames = []gwv1.Hostname{"*"}
		}
		if len(parent.Hostnames) > 0 {
			// TODO: the spec actually has a label match, not a string match. That is, *.com does not match *.apple.com
			// We are doing a string match here
			matched := false
			hostMatched := false
		out:
			for _, routeHostname := range hostnames {
				for _, parentHostNamespace := range parent.Hostnames {
					var parentNamespace, parentHostname string
					// When parentHostNamespace lacks a '/', it was likely sanitized from '*/host' to 'host'
					// by sanitizeServerHostNamespace. Set parentNamespace to '*' to reflect the wildcard namespace
					// and parentHostname to the sanitized host to prevent an index out of range panic.
					if strings.Contains(parentHostNamespace, "/") {
						spl := strings.Split(parentHostNamespace, "/")
						parentNamespace, parentHostname = spl[0], spl[1]
					} else {
						parentNamespace, parentHostname = "*", parentHostNamespace
					}

					hostnameMatch := host.Name(parentHostname).Matches(host.Name(routeHostname))
					namespaceMatch := parentNamespace == "*" || parentNamespace == localNamespace
					hostMatched = hostMatched || hostnameMatch
					if hostnameMatch && namespaceMatch {
						matched = true
						break out
					}
				}
			}
			if !matched {
				if hostMatched {
					return &ParentError{
						Reason: ParentErrorNotAllowed,
						Message: fmt.Sprintf(
							"hostnames matched parent hostname %q, but namespace %q is not allowed by the parent",
							parent.OriginalHostname, localNamespace,
						),
					}
				}
				return &ParentError{
					Reason: ParentErrorNoHostname,
					Message: fmt.Sprintf(
						"no hostnames matched parent hostname %q",
						parent.OriginalHostname,
					),
				}
			}
		}
	}
	// Also make sure this route kind is allowed
	matched := false
	for _, ak := range parent.AllowedKinds {
		if string(ak.Kind) == routeKind.Kind && ptr.OrDefault((*string)(ak.Group), gvk.GatewayClass.Group) == routeKind.Group {
			matched = true
			break
		}
	}
	if !matched {
		return &ParentError{
			Reason:  ParentErrorNotAllowed,
			Message: fmt.Sprintf("kind %v is not allowed", routeKind),
		}
	}
	return nil
}

func extractParentReferenceInfo(ctx RouteContext, parents RouteParents, obj controllers.Object) []routeParentReference {
	routeRefs, hostnames, kind := GetCommonRouteInfo(obj)
	localNamespace := obj.GetNamespace()
	var parentRefs []routeParentReference
	for _, ref := range routeRefs {
		ir, err := toInternalParentReference(ref, localNamespace)
		if err != nil {
			// Cannot handle the reference. Maybe it is for another controller, so we just ignore it
			continue
		}
		pk := parentReference{
			parentKey:   ir,
			SectionName: ptr.OrEmpty(ref.SectionName),
			Port:        ptr.OrEmpty(ref.Port),
		}
		gk := ir
		currentParents := parents.fetch(ctx.Krt, gk)
		appendParent := func(pr *parentInfo, pk parentReference) {
			bannedHostnames := sets.New[string]()
			for _, gw := range currentParents {
				if gw == pr {
					continue // do not ban ourself
				}
				if gw.Port != pr.Port {
					// We only care about listeners on the same port
					continue
				}
				if gw.Protocol != pr.Protocol {
					// We only care about listeners on the same protocol
					continue
				}
				bannedHostnames.Insert(gw.OriginalHostname)
			}
			deniedReason := referenceAllowed(ctx, pr, kind, pk, hostnames, localNamespace)
			rpi := routeParentReference{
				InternalName:      pr.InternalName,
				InternalKind:      ir.Kind,
				Hostname:          pr.OriginalHostname,
				DeniedReason:      deniedReason,
				OriginalReference: ref,
				BannedHostnames:   bannedHostnames.Copy().Delete(pr.OriginalHostname),
				ParentKey:         ir,
				ParentSection:     pr.SectionName,
			}
			parentRefs = append(parentRefs, rpi)
		}
		for _, gw := range currentParents {
			// Append all matches. Note we may be adding mismatch section or ports; this is handled later
			appendParent(gw, pk)
		}
	}
	// Ensure stable order
	slices.SortBy(parentRefs, func(a routeParentReference) string {
		return parentRefString(a.OriginalReference)
	})
	return parentRefs
}

// https://github.com/kubernetes-sigs/gateway-api/blob/cea484e38e078a2c1997d8c7a62f410a1540f519/apis/v1beta1/httproute_types.go#L207-L212
func isInvalidBackend(err *reporter.RouteCondition) bool {
	return err.Reason == gwv1.RouteReasonRefNotPermitted ||
		err.Reason == gwv1.RouteReasonBackendNotFound ||
		err.Reason == gwv1.RouteReasonInvalidKind
}

// parentKey holds info about a parentRef (eg route binding to a Gateway). This is a mirror of
// gwv1.ParentReference in a form that can be stored in a map
type parentKey struct {
	Kind schema.GroupVersionKind
	// Name is the original name of the resource (eg Kubernetes Gateway name)
	Name string
	// Namespace is the namespace of the resource
	Namespace string
}

func (p parentKey) String() string {
	return p.Kind.String() + "/" + p.Namespace + "/" + p.Name
}

type parentReference struct {
	parentKey

	SectionName gwv1.SectionName
	Port        gwv1.PortNumber
}

func (p parentReference) String() string {
	return p.parentKey.String() + "/" + string(p.SectionName) + "/" + fmt.Sprint(p.Port)
}

// parentInfo holds info about a "parent" - something that can be referenced as a ParentRef in the API.
// Today, this is just Gateway
type parentInfo struct {
	// InternalName refers to the internal name we can reference it by. For example "my-ns/my-gateway"
	InternalName string
	// AllowedKinds indicates which kinds can be admitted by this parent
	AllowedKinds []gwv1.RouteGroupKind
	// Hostnames is the hostnames that must be match to reference to the parent. For gateway this is listener hostname
	// Format is ns/hostname
	Hostnames []string
	// OriginalHostname is the unprocessed form of Hostnames; how it appeared in users' config
	OriginalHostname string

	SectionName gwv1.SectionName
	Port        gwv1.PortNumber
	Protocol    gwv1.ProtocolType
}

// routeParentReference holds information about a route's parent reference
type routeParentReference struct {
	// InternalName refers to the internal name of the parent we can reference it by. For example "my-ns/my-gateway"
	InternalName string
	// InternalKind is the Group/Kind of the parent
	InternalKind schema.GroupVersionKind
	// DeniedReason, if present, indicates why the reference was not valid
	DeniedReason *ParentError
	// OriginalReference contains the original reference
	OriginalReference gwv1.ParentReference
	// Hostname is the hostname match of the parent, if any
	Hostname        string
	BannedHostnames sets.Set[string]
	ParentKey       parentKey
	ParentSection   gwv1.SectionName
}

func filteredReferences(parents []routeParentReference) []routeParentReference {
	ret := make([]routeParentReference, 0, len(parents))
	for _, p := range parents {
		if p.DeniedReason != nil {
			// We should filter this out
			continue
		}
		ret = append(ret, p)
	}
	// To ensure deterministic order, sort them
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].InternalName < ret[j].InternalName
	})
	return ret
}

func getDefaultName(name string, kgw *gwv1.GatewaySpec) string {
	return fmt.Sprintf("%v-%v", name, kgw.GatewayClassName)
}

// IsManaged checks if a Gateway is managed (ie we create the Deployment and Service) or unmanaged.
// This is based on the address field of the spec. If address is set with a Hostname type, it should point to an existing
// Service that handles the gateway traffic. If it is not set, or refers to only a single IP, we will consider it managed and provision the Service.
// If there is an IP, we will set the `loadBalancerIP` type.
// While there is no defined standard for this in the API yet, it is tracked in https://github.com/kubernetes-sigs/gateway-api/issues/892.
// So far, this mirrors how out of clusters work (address set means to use existing IP, unset means to provision one),
// and there has been growing consensus on this model for in cluster deployments.
//
// Currently, the supported options are:
// * 1 Hostname value. This can be short Service name ingress, or FQDN ingress.ns.svc.cluster.local, example.com. If its a non-k8s FQDN it is a ServiceEntry.
// * 1 IP address. This is managed, with IP explicit
// * Nothing. This is managed, with IP auto assigned
//
// Not supported:
// Multiple hostname/IP - It is feasible but preference is to create multiple Gateways. This would also break the 1:1 mapping of GW:Service
// Mixed hostname and IP - doesn't make sense; user should define the IP in service
// NamedAddress - Service has no concept of named address. For cloud's that have named addresses they can be configured by annotations,
//
//	which users can add to the Gateway.
//
// If manual deployments are disabled, IsManaged() always returns true.
func IsManaged(gw *gwv1.GatewaySpec) bool {
	//if !features.EnableGatewayAPIManualDeployment {
	//	return true
	//}
	if len(gw.Addresses) == 0 {
		return true
	}
	if len(gw.Addresses) > 1 {
		return false
	}
	if t := gw.Addresses[0].Type; t == nil || *t == gwv1.IPAddressType {
		return true
	}
	return false
}

func extractGatewayServices(domainSuffix string, kgw *gwv1.Gateway) ([]string, *reporter.RouteCondition) {
	if IsManaged(&kgw.Spec) {
		name := model.GetOrDefault(kgw.Annotations[annotation.GatewayNameOverride.Name], getDefaultName(kgw.Name, &kgw.Spec))
		return []string{fmt.Sprintf("%s.%s.svc.%v", name, kgw.Namespace, domainSuffix)}, nil
	}
	gatewayServices := []string{}
	skippedAddresses := []string{}
	for _, addr := range kgw.Spec.Addresses {
		if addr.Type != nil && *addr.Type != gwv1.HostnameAddressType {
			// We only support HostnameAddressType. Keep track of invalid ones so we can report in status.
			skippedAddresses = append(skippedAddresses, addr.Value)
			continue
		}
		// TODO: For now we are using Addresses. There has been some discussion of allowing inline
		// parameters on the class field like a URL, in which case we will probably just use that. See
		// https://github.com/kubernetes-sigs/gateway-api/pull/614
		fqdn := addr.Value
		if !strings.Contains(fqdn, ".") {
			// Short name, expand it
			fqdn = fmt.Sprintf("%s.%s.svc.%s", fqdn, kgw.Namespace, domainSuffix)
		}
		gatewayServices = append(gatewayServices, fqdn)
	}
	if len(skippedAddresses) > 0 {
		// Give error but return services, this is a soft failure
		return gatewayServices, &reporter.RouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonUnsupportedValue,
			Message: fmt.Sprintf("only Hostname is supported, ignoring %v", skippedAddresses),
		}
	}
	if _, f := kgw.Annotations[annotation.NetworkingServiceType.Name]; f {
		// Give error but return services, this is a soft failure
		// Remove entirely in 1.20
		return gatewayServices, &reporter.RouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonUnsupportedValue,
			Message: fmt.Sprintf("annotation %v is deprecated, use Spec.Infrastructure.Routeability", annotation.NetworkingServiceType.Name),
		}
	}
	return gatewayServices, nil
}

func buildListener(
	ctx krt.HandlerContext,
	secrets krt.Collection[*corev1.Secret],
	grants ReferenceGrants,
	namespaces krt.Collection[*corev1.Namespace],
	obj *gwv1.Gateway,
	status *gwv1.GatewayStatus,
	l gwv1.Listener,
	listenerIndex int,
	controllerName gwv1.GatewayController,
) (*istio.Server, *TLSInfo, bool) {
	listenerConditions := map[string]*condition{
		string(gwv1.ListenerConditionAccepted): {
			reason:  string(gwv1.ListenerReasonAccepted),
			message: "No errors found",
		},
		string(gwv1.ListenerConditionProgrammed): {
			reason:  string(gwv1.ListenerReasonProgrammed),
			message: "No errors found",
		},
		string(gwv1.ListenerConditionConflicted): {
			reason:  string(gwv1.ListenerReasonNoConflicts),
			message: "No errors found",
			status:  kstatus.StatusFalse,
		},
		string(gwv1.ListenerConditionResolvedRefs): {
			reason:  string(gwv1.ListenerReasonResolvedRefs),
			message: "No errors found",
		},
	}

	ok := true
	tls, tlsInfo, err := buildTLS(ctx, secrets, grants, l.TLS, obj, kube.IsAutoPassthrough(obj.Labels, l))
	if err != nil {
		listenerConditions[string(gwv1.ListenerConditionResolvedRefs)].error = err
		listenerConditions[string(gwv1.GatewayConditionProgrammed)].error = &ConfigError{
			Reason:  string(gwv1.GatewayReasonInvalid),
			Message: "Bad TLS configuration",
		}
		ok = false
	}

	hostnames := buildHostnameMatch(ctx, obj.Namespace, namespaces, l)
	protocol, perr := listenerProtocolToAgentgateway(controllerName, l.Protocol)
	if perr != nil {
		listenerConditions[string(gwv1.ListenerConditionAccepted)].error = &ConfigError{
			Reason:  string(gwv1.ListenerReasonUnsupportedProtocol),
			Message: perr.Error(),
		}
		ok = false
	}
	server := &istio.Server{
		Port: &istio.Port{
			// Name is required. We only have one server per GatewayListener, so we can just name them all the same
			Name:     "default",
			Number:   uint32(l.Port),
			Protocol: protocol,
		},
		Hosts: hostnames,
		Tls:   tls,
	}

	reportListenerCondition(listenerIndex, l, obj, status, listenerConditions)
	return server, tlsInfo, ok
}

var supportedProtocols = sets.New(
	gwv1.HTTPProtocolType,
	gwv1.HTTPSProtocolType,
	gwv1.TLSProtocolType,
	gwv1.TCPProtocolType,
	gwv1.ProtocolType(protocol.HBONE))

func listenerProtocolToAgentgateway(name gwv1.GatewayController, p gwv1.ProtocolType) (string, error) {
	switch p {
	// Standard protocol types
	case gwv1.HTTPProtocolType:
		return string(p), nil
	case gwv1.HTTPSProtocolType:
		return string(p), nil
	case gwv1.TLSProtocolType, gwv1.TCPProtocolType:
		// TODO: check if TLS/TCP alpha features are supported
		return string(p), nil
	}
	up := gwv1.ProtocolType(strings.ToUpper(string(p)))
	if supportedProtocols.Contains(up) {
		return "", fmt.Errorf("protocol %q is unsupported. hint: %q (uppercase) may be supported", p, up)
	}
	// Note: the k8s.UDPProtocolType is explicitly left to hit this path
	return "", fmt.Errorf("protocol %q is unsupported", p)
}

func buildTLS(
	ctx krt.HandlerContext,
	secrets krt.Collection[*corev1.Secret],
	grants ReferenceGrants,
	tls *gwv1.GatewayTLSConfig,
	gw *gwv1.Gateway,
	isAutoPassthrough bool,
) (*istio.ServerTLSSettings, *TLSInfo, *ConfigError) {
	if tls == nil {
		return nil, nil, nil
	}
	// Explicitly not supported: file mounted
	// Not yet implemented: TLS mode, https redirect, max protocol version, SANs, CipherSuites, VerifyCertificate
	out := &istio.ServerTLSSettings{
		HttpsRedirect: false,
	}
	mode := gwv1.TLSModeTerminate
	if tls.Mode != nil {
		mode = *tls.Mode
	}
	namespace := gw.Namespace
	switch mode {
	case gwv1.TLSModeTerminate:
		out.Mode = istio.ServerTLSSettings_SIMPLE
		if tls.Options != nil {
			switch tls.Options[gatewayTLSTerminateModeKey] {
			case "MUTUAL":
				out.Mode = istio.ServerTLSSettings_MUTUAL
			case "ISTIO_MUTUAL":
				out.Mode = istio.ServerTLSSettings_ISTIO_MUTUAL
				return out, nil, nil
			}
		}
		if len(tls.CertificateRefs) != 1 {
			// This is required in the API, should be rejected in validation
			return out, nil, &ConfigError{Reason: InvalidTLS, Message: "exactly 1 certificateRefs should be present for TLS termination"}
		}
		cred, tlsInfo, err := buildSecretReference(ctx, tls.CertificateRefs[0], gw, secrets)
		if err != nil {
			return out, nil, err
		}
		credNs := ptr.OrDefault((*string)(tls.CertificateRefs[0].Namespace), namespace)
		sameNamespace := credNs == namespace
		if !sameNamespace && !grants.SecretAllowed(ctx, creds.ToResourceName(cred), namespace) {
			return out, nil, &ConfigError{
				Reason: InvalidListenerRefNotPermitted,
				Message: fmt.Sprintf(
					"certificateRef %v/%v not accessible to a Gateway in namespace %q (missing a ReferenceGrant?)",
					tls.CertificateRefs[0].Name, credNs, namespace,
				),
			}
		}
		out.CredentialName = cred
		return out, &tlsInfo, nil
	case gwv1.TLSModePassthrough:
		out.Mode = istio.ServerTLSSettings_PASSTHROUGH
		if isAutoPassthrough {
			out.Mode = istio.ServerTLSSettings_AUTO_PASSTHROUGH
		}
	}
	return out, nil, nil
}

func buildSecretReference(
	ctx krt.HandlerContext,
	ref gwv1.SecretObjectReference,
	gw *gwv1.Gateway,
	secrets krt.Collection[*corev1.Secret],
) (string, TLSInfo, *ConfigError) {
	if normalizeReference(ref.Group, ref.Kind, wellknown.SecretGVK) != wellknown.SecretGVK {
		return "", TLSInfo{}, &ConfigError{Reason: InvalidTLS, Message: fmt.Sprintf("invalid certificate reference %v, only secret is allowed", objectReferenceString(ref))}
	}

	secret := ConfigKey{
		Kind:      kind.Secret,
		Name:      string(ref.Name),
		Namespace: ptr.OrDefault((*string)(ref.Namespace), gw.Namespace),
	}

	key := secret.Namespace + "/" + secret.Name
	scrt := ptr.Flatten(krt.FetchOne(ctx, secrets, krt.FilterKey(key)))
	if scrt == nil {
		return "", TLSInfo{}, &ConfigError{
			Reason:  InvalidTLS,
			Message: fmt.Sprintf("invalid certificate reference %v, secret %v not found", objectReferenceString(ref), key),
		}
	}
	certInfo, err := kubecreds.ExtractCertInfo(scrt)
	if err != nil {
		return "", TLSInfo{}, &ConfigError{
			Reason:  InvalidTLS,
			Message: fmt.Sprintf("invalid certificate reference %v, %v", objectReferenceString(ref), err),
		}
	}
	if _, err = tls.X509KeyPair(certInfo.Cert, certInfo.Key); err != nil {
		return "", TLSInfo{}, &ConfigError{
			Reason:  InvalidTLS,
			Message: fmt.Sprintf("invalid certificate reference %v, the certificate is malformed: %v", objectReferenceString(ref), err),
		}
	}
	return creds.ToKubernetesGatewayResource(secret.Namespace, secret.Name), TLSInfo{
		Cert: certInfo.Cert,
		Key:  certInfo.Key,
	}, nil
}

func objectReferenceString(ref gwv1.SecretObjectReference) string {
	return fmt.Sprintf("%s/%s/%s.%s",
		ptr.OrEmpty(ref.Group),
		ptr.OrEmpty(ref.Kind),
		ref.Name,
		ptr.OrEmpty(ref.Namespace))
}

func parentRefString(ref gwv1.ParentReference) string {
	return fmt.Sprintf("%s/%s/%s/%s/%d.%s",
		ptr.OrEmpty(ref.Group),
		ptr.OrEmpty(ref.Kind),
		ref.Name,
		ptr.OrEmpty(ref.SectionName),
		ptr.OrEmpty(ref.Port),
		ptr.OrEmpty(ref.Namespace))
}

// buildHostnameMatch generates a Gateway.spec.servers.hosts section from a listener
func buildHostnameMatch(ctx krt.HandlerContext, localNamespace string, namespaces krt.Collection[*corev1.Namespace], l gwv1.Listener) []string {
	// We may allow all hostnames or a specific one
	hostname := "*"
	if l.Hostname != nil {
		hostname = string(*l.Hostname)
	}

	resp := []string{}
	for _, ns := range namespacesFromSelector(ctx, localNamespace, namespaces, l.AllowedRoutes) {
		// This check is necessary to prevent adding a hostname with an invalid empty namespace
		if len(ns) > 0 {
			resp = append(resp, fmt.Sprintf("%s/%s", ns, hostname))
		}
	}

	// If nothing matched use ~ namespace (match nothing). We need this since its illegal to have an
	// empty hostname list, but we still need the Gateway provisioned to ensure status is properly set and
	// SNI matches are established; we just don't want to actually match any routing rules (yet).
	if len(resp) == 0 {
		return []string{"~/" + hostname}
	}
	return resp
}

// namespacesFromSelector determines a list of allowed namespaces for a given AllowedRoutes
func namespacesFromSelector(ctx krt.HandlerContext, localNamespace string, namespaceCol krt.Collection[*corev1.Namespace], lr *gwv1.AllowedRoutes) []string {
	// Default is to allow only the same namespace
	if lr == nil || lr.Namespaces == nil || lr.Namespaces.From == nil || *lr.Namespaces.From == gwv1.NamespacesFromSame {
		return []string{localNamespace}
	}
	if *lr.Namespaces.From == gwv1.NamespacesFromAll {
		return []string{"*"}
	}

	if lr.Namespaces.Selector == nil {
		// Should never happen, invalid config
		return []string{"*"}
	}

	// gateway-api has selectors, but Istio Gateway just has a list of names. We will run the selector
	// against all namespaces and get a list of matching namespaces that can be converted into a list
	// Istio can handle.
	ls, err := metav1.LabelSelectorAsSelector(lr.Namespaces.Selector)
	if err != nil {
		return nil
	}
	namespaces := []string{}
	namespaceObjects := krt.Fetch(ctx, namespaceCol)
	for _, ns := range namespaceObjects {
		if ls.Matches(toNamespaceSet(ns.Name, ns.Labels)) {
			namespaces = append(namespaces, ns.Name)
		}
	}
	// Ensure stable order
	sort.Strings(namespaces)
	return namespaces
}

// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
const NamespaceNameLabel = "kubernetes.io/metadata.name"

// toNamespaceSet converts a set of namespace labels to a Set that can be used to select against.
func toNamespaceSet(name string, labels map[string]string) klabels.Set {
	// If namespace label is not set, implicitly insert it to support older Kubernetes versions
	if labels[NamespaceNameLabel] == name {
		// Already set, avoid copies
		return labels
	}
	// First we need a copy to not modify the underlying object
	ret := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		ret[k] = v
	}
	ret[NamespaceNameLabel] = name
	return ret
}

func GetCommonRouteInfo(spec any) ([]gwv1.ParentReference, []gwv1.Hostname, schema.GroupVersionKind) {
	switch t := spec.(type) {
	case *gwv1alpha2.TCPRoute:
		return t.Spec.ParentRefs, nil, wellknown.TCPRouteGVK
	case *gwv1alpha2.TLSRoute:
		return t.Spec.ParentRefs, t.Spec.Hostnames, wellknown.TLSRouteGVK
	case *gwv1.HTTPRoute:
		return t.Spec.ParentRefs, t.Spec.Hostnames, wellknown.HTTPRouteGVK
	case *gwv1beta1.HTTPRoute:
		return t.Spec.ParentRefs, t.Spec.Hostnames, wellknown.HTTPRouteGVK
	case *gwv1.GRPCRoute:
		return t.Spec.ParentRefs, t.Spec.Hostnames, wellknown.GRPCRouteGVK
	default:
		log.Fatalf("unknown type %T", t)
		return nil, nil, schema.GroupVersionKind{}
	}
}

func defaultString[T ~string](s *T, def string) string {
	if s == nil {
		return def
	}
	return string(*s)
}

func toRouteKind(g schema.GroupVersionKind) gwv1.RouteGroupKind {
	return gwv1.RouteGroupKind{Group: (*gwv1.Group)(&g.Group), Kind: gwv1.Kind(g.Kind)}
}

func routeGroupKindEqual(rgk1, rgk2 gwv1.RouteGroupKind) bool {
	return rgk1.Kind == rgk2.Kind && getGroup(rgk1) == getGroup(rgk2)
}

func getGroup(rgk gwv1.RouteGroupKind) gwv1.Group {
	return ptr.OrDefault(rgk.Group, wellknown.GatewayGroup)
}
