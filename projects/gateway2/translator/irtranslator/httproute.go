package irtranslator

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
)

// TODO (danehans): Rename this file to route.go since it supports different route types.

// RouteInfo contains pre-resolved backends (Services, Upstreams and delegated xRoutes)
// This allows all querying to happen upfront, and detailed logic for delegation to happen
// as part of translation.
type RouteInfo struct {
	// Object is the generic route object which could be HTTPRoute, TCPRoute, etc.
	Object ir.Route

	// ParentRef points to the Gateway (and optionally Listener) or HTTPRoute.
	ParentRef gwv1.ParentReference

	// ParentRef points to the Gateway (and optionally Listener).
	ListenerParentRef gwv1.ParentReference

	// hostnameOverrides can replace the HTTPRoute hostnames with those that intersect
	// the attached listener's hostname(s).
	HostnameOverrides []string

	// Children contains all delegate HTTPRoutes referenced in any rule of this
	// HTTPRoute, keyed by the backend ref for easy lookup.
	// This tree structure can have cyclic references. Check them when recursing through the tree.
	Children BackendMap[[]*RouteInfo]
}

// GetKind returns the kind of the route.
func (r RouteInfo) GetKind() string {
	return r.Object.GetGroupKind().Kind
}

// GetName returns the name of the route.
func (r RouteInfo) GetName() string {
	return r.Object.GetName()
}

// GetNamespace returns the namespace of the route.
func (r RouteInfo) GetNamespace() string {
	return r.Object.GetNamespace()
}

// Hostnames returns the hostname overrides if they exist, otherwise it returns
// the hostnames specified in the HTTPRoute.
func (r *RouteInfo) Hostnames() []string {
	if len(r.HostnameOverrides) > 0 {
		return r.HostnameOverrides
	}

	httpRoute, ok := r.Object.(*ir.HttpRouteIR)
	if !ok {
		return []string{}
	}

	strs := make([]string, 0, len(httpRoute.Hostnames))
	for _, v := range httpRoute.Hostnames {
		strs = append(strs, string(v))
	}
	return strs
}

// GetChildrenForRef fetches child routes for a given BackendObjectReference.
func (r *RouteInfo) GetChildrenForRef(backendRef ir.ObjectSource) ([]*RouteInfo, error) {
	return r.Children.Get(backendRef, nil)
}

// Clone creates a deep copy of the RouteInfo object.
func (r *RouteInfo) Clone() *RouteInfo {
	if r == nil {
		return nil
	}
	// TODO (danehans): Why are hostnameOverrides not being cloned?
	return &RouteInfo{
		Object:    r.Object,
		ParentRef: r.ParentRef,
		Children:  r.Children,
	}
}

// UniqueRouteName returns a unique name for the route based on the route kind, name, namespace,
// and the given indexes.
func (r *RouteInfo) UniqueRouteName(ruleIdx, matchIdx int) string {
	return fmt.Sprintf("%s-%s-%s-%d-%d", strings.ToLower(r.GetKind()), r.GetName(), r.GetNamespace(), ruleIdx, matchIdx)
}

// ENTRYPOINT!!
// GetRouteChain recursively resolves all backends for the given route object.
// It handles delegation of HTTPRoutes and resolves child routes.
func (r *gatewayQueries) GetRouteChain(kctx krt.HandlerContext,
	ctx context.Context,
	route ir.Route,
	hostnames []string,
	parentRef gwv1.ParentReference,
) *RouteInfo {
	var children BackendMap[[]*RouteInfo]

	switch typedRoute := route.(type) {
	case *ir.HttpRouteIR:
		panic("TODO: add errors if we have errors on the backends")
		children = r.getDelegatedChildren(kctx, ctx, typedRoute, parentRef, nil)
	case *ir.TcpRouteIR:
		panic("TODO: add errors if we have errors on the backends")
		// TODO (danehans): Should TCPRoute delegation support be added in the future?
	default:
		return nil
	}

	return &RouteInfo{
		Object:            route,
		HostnameOverrides: hostnames,
		ParentRef:         parentRef,
		Children:          children,
	}
}

func (r *gatewayQueries) allowedRoutes(gw *gwv1.Gateway, l *gwv1.Listener) (func(string) bool, []metav1.GroupKind, error) {
	var allowedKinds []metav1.GroupKind

	// Determine the allowed route kinds based on the listener's protocol
	switch l.Protocol {
	case gwv1.HTTPSProtocolType:
		fallthrough
	case gwv1.HTTPProtocolType:
		allowedKinds = []metav1.GroupKind{{Kind: wellknown.HTTPRouteKind, Group: gwv1.GroupName}}
	case gwv1.TLSProtocolType:
		fallthrough
	case gwv1.TCPProtocolType:
		allowedKinds = []metav1.GroupKind{{Kind: wellknown.TCPRouteKind, Group: gwv1a2.GroupName}}
	case gwv1.UDPProtocolType:
		allowedKinds = []metav1.GroupKind{{}}
	default:
		// allow custom protocols to work
		allowedKinds = []metav1.GroupKind{{Kind: wellknown.HTTPRouteKind, Group: gwv1.GroupName}}
	}

	allowedNs := SameNamespace(gw.Namespace)
	if ar := l.AllowedRoutes; ar != nil {
		// Override the allowed route kinds if specified in AllowedRoutes
		if ar.Kinds != nil {
			allowedKinds = nil // Reset to include only explicitly allowed kinds
			for _, k := range ar.Kinds {
				gk := metav1.GroupKind{Kind: string(k.Kind)}
				if k.Group != nil {
					gk.Group = string(*k.Group)
				} else {
					gk.Group = gwv1.GroupName
				}
				allowedKinds = append(allowedKinds, gk)
			}
		}

		// Determine the allowed namespaces if specified
		if ar.Namespaces != nil && ar.Namespaces.From != nil {
			switch *ar.Namespaces.From {
			case gwv1.NamespacesFromAll:
				allowedNs = AllNamespace()
			case gwv1.NamespacesFromSelector:
				if ar.Namespaces.Selector == nil {
					return nil, nil, fmt.Errorf("selector must be set")
				}
				selector, err := metav1.LabelSelectorAsSelector(ar.Namespaces.Selector)
				if err != nil {
					return nil, nil, err
				}
				allowedNs = r.NamespaceSelector(selector)
			}
		}
	}

	return allowedNs, allowedKinds, nil
}

func (r *gatewayQueries) getDelegatedChildren(kctx krt.HandlerContext,
	ctx context.Context,
	parent *ir.HttpRouteIR,
	listenerRef gwv1.ParentReference,
	visited sets.Set[types.NamespacedName],
) BackendMap[[]*RouteInfo] {
	// Initialize the set of visited routes if it hasn't been initialized yet
	if visited == nil {
		visited = sets.New[types.NamespacedName]()
	}
	parentRef := namespacedName(parent)
	visited.Insert(parentRef)

	children := NewBackendMap[[]*RouteInfo]()
	for _, parentRule := range parent.Rules {
		var refChildren []*RouteInfo
		for _, backendRef := range parentRule.Backends {
			// Check if the backend reference is an HTTPRoute
			if backendRef.Delegate == nil {
				continue
			}
			ref := *backendRef.Delegate
			// Fetch child routes based on the backend reference
			referencedRoutes, err := r.fetchChildRoutes(kctx, ctx, backendRef)
			if err != nil {
				children.AddError(ref, err)
				continue
			}
			for _, childRoute := range referencedRoutes {
				childRef := namespacedName(&childRoute)
				if visited.Has(childRef) {
					err := fmt.Errorf("ignoring child route %s for parent %s: %w", parentRef, childRef, ErrCyclicReference)
					children.AddError(ref, err)
					// Don't resolve invalid child route
					continue
				}
				// Recursively get the route chain for each child route
				routeInfo := &RouteInfo{
					Object: &childRoute,
					ParentRef: gwv1.ParentReference{
						Group:     ptr.To(gwv1.Group(wellknown.GatewayGroup)),
						Kind:      ptr.To(gwv1.Kind(wellknown.HTTPRouteKind)),
						Namespace: ptr.To(gwv1.Namespace(parent.Namespace)),
						Name:      gwv1.ObjectName(parent.Name),
					},
					ListenerParentRef: listenerRef,
					Children:          r.getDelegatedChildren(kctx, ctx, &childRoute, listenerRef, visited),
				}
				refChildren = append(refChildren, routeInfo)
			}
			// Add the resolved children routes to the backend map
			children.Add(ref, refChildren)
		}
	}
	return children
}

func (r *gatewayQueries) fetchChildRoutes(
	kctx krt.HandlerContext,
	ctx context.Context,
	backend ir.HttpBackendOrDelegate,
) ([]ir.HttpRouteIR, error) {
	if backend.Delegate == nil {
		return nil, nil
	}
	backendRef := *backend.Delegate
	delegatedNs := backendRef.Namespace

	var refChildren []ir.HttpRouteIR
	if string(backendRef.Name) == "" || string(backendRef.Name) == "*" {
		// Handle wildcard references by listing all HTTPRoutes in the specified namespace
		routes := r.routes.ListHttp(kctx, delegatedNs)
		refChildren = append(refChildren, routes...)
		panic("TODO: give krt context")
	} else {
		// Lookup a specific child route by its name
		route := r.routes.FetchHttp(kctx, delegatedNs, string(backendRef.Name))
		if route == nil {
			return nil, errors.New("not found")
		}
		refChildren = append(refChildren, *route)
		panic("TODO: give krt context")
	}
	// Check if no child routes were resolved and log an error if needed
	if len(refChildren) == 0 {
		return nil, ErrUnresolvedReference
	}

	return refChildren, nil
}

// ENTRYPOINT!!
// this is projects/gateway2/translator/listener/gateway_listener_translator.go#buildRoutesPerHost()
func (q *gatewayQueries) GetFlattenedRoutes(routeInfos []*RouteInfo) {
	for _, routeWithHosts := range routeInfos {
		// TODO: reporter

		//		// Only HTTPRoute types should be translated.
		//		_, ok := routeWithHosts.Object.(*gwv1.HTTPRoute)
		//		if !ok {
		//			// TODO:
		//		}

		routes := translateRouteRules(routeWithHosts)

		if len(routes) == 0 {
			// TODO report
			continue
		}

		hostnames := routeWithHosts.Hostnames()
		if len(hostnames) == 0 {
			hostnames = []string{"*"}
		}

		//		for _, host := range hostnames {
		//TODO:		 	routesByHost[host] = append(routesByHost[host], routeutils.ToSortable(routeWithHosts.Object, routes)...)
		//		}
	}
}

// ENTRYPOINT!!!!!!!!!!!!!!!!!!!!
// this is projects/gateway2/translator/httproute/gateway_http_route_translator.go#TranslateGatewayHTTPRouteRules
func translateRouteRules(
	// gwListener gwv1.Listener,
	routeInfo *RouteInfo,
) []*ir.HttpRouteRuleMatchIR {
	var finalRoutes []*ir.HttpRouteRuleMatchIR

	// Only HTTPRoute types should be translated.
	route, ok := routeInfo.Object.(*ir.HttpRouteIR)
	if !ok {
		return finalRoutes
	}

	// Hostnames need to be explicitly passed to the plugins since they
	// are required by delegatee (child) routes of delegated routes that
	// won't have spec.Hostnames set.
	hostnames := make([]string, len(route.Hostnames))
	copy(hostnames, route.Hostnames)

	routesVisited := sets.New[types.NamespacedName]()
	delegationChain := list.New()

	translateRouteRulesUtil(
		routeInfo, &finalRoutes, routesVisited, hostnames, delegationChain)
	return finalRoutes
}

func translateRouteRulesUtil(
	// gwListener gwv1.Listener,
	routeInfo *RouteInfo,
	outputs *[]*ir.HttpRouteRuleMatchIR,
	routesVisited sets.Set[types.NamespacedName],
	hostnames []string,
	delegationChain *list.List,
) {
	// this is already done earlier, maybe consolidate
	route, ok := routeInfo.Object.(*ir.HttpRouteIR)
	if !ok {
		return
	}

	for ruleIdx, rule := range route.Rules {
		rule := rule
		if rule.Matches == nil {
			// from the spec:
			// If no matches are specified, the default is a prefix path match on “/”, which has the effect of matching every HTTP request.
			rule.Matches = []gwv1.HTTPRouteMatch{{}}
		}

		outputRoutes := translateGatewayHTTPRouteRule(
			routeInfo,
			rule,
			ruleIdx,
			outputs,
			routesVisited,
			hostnames,
			delegationChain,
		)
		for _, outputRoute := range outputRoutes {
			// The above function will return a nil route if it delegates and thus we have no actual route to add
			if outputRoute == nil {
				continue
			}

			*outputs = append(*outputs, outputRoute)
		}
	}
}

func translateGatewayHTTPRouteRule(
	// gwListener gwv1.Listener,
	gwroute *RouteInfo,
	rule ir.HttpRouteRuleIR,
	ruleIdx int,
	outputs *[]*ir.HttpRouteRuleMatchIR,
	routesVisited sets.Set[types.NamespacedName],
	hostnames []string,
	delegationChain *list.List,
) []*ir.HttpRouteRuleMatchIR {
	routes := make([]*ir.HttpRouteRuleMatchIR, len(rule.Matches))
	// Only HTTPRoutes should be translated.
	_, ok := gwroute.Object.(*ir.HttpRouteIR)
	if !ok {
		return routes
	}

	for idx, match := range rule.Matches {
		match := match // pike
		// HTTPRoute names are being introduced to upstream as part of https://github.com/kubernetes-sigs/gateway-api/issues/995
		// For now, the HTTPRoute needs a unique name for each Route to support features that require the route name
		// set (basic ratelimit, route-level jwt, etc.). The unique name is generated by appending the index of the route to the
		// HTTPRoute name.namespace.
		uniqueRouteName := gwroute.UniqueRouteName(ruleIdx, idx)
		var outputRoute ir.HttpRouteRuleMatchIR
		outputRoute.HttpRouteRuleCommonIR = rule.HttpRouteRuleCommonIR
		outputRoute.ParentRef = gwroute.ListenerParentRef
		outputRoute.Name = uniqueRouteName
		outputRoute.Backends = nil
		outputRoute.MatchIndex = idx
		outputRoute.Match = match

		var delegatedRoutes []*ir.HttpRouteRuleMatchIR
		var delegates bool
		if len(rule.Backends) > 0 {
			delegates = setRouteAction(
				gwroute,
				rule,
				&outputRoute,
				outputRoute.Match,
				&delegatedRoutes,
				routesVisited,
				delegationChain,
			)
		}

		// TODO (major todo!): handle attachment processing

		// rtCtx := &plugins.RouteContext{
		// 	// Listener:        &gwListener,
		// 	HTTPRoute:       route,
		// 	Hostnames:       hostnames,
		// 	DelegationChain: delegationChain,
		// 	Rule:            &rule,
		// 	Match:           &match,
		// 	// Reporter:        reporter,
		// }

		// Apply the plugins for this route
		// for _, plugin := range pluginRegistry.GetRoutePlugins() {
		// 	err := plugin.ApplyRoutePlugin(ctx, rtCtx, outputRoute)
		// 	if err != nil {
		// 		contextutils.LoggerFrom(ctx).Errorf("error in RoutePlugin: %v", err)
		// 	}

		// 	// If this parent route has delegatee routes, override any applied policies
		// 	// that are on the child with the parent's policies.
		// 	// When a plugin is invoked on a route, it must override the existing route.
		// 	for _, child := range delegatedRoutes {
		// 		err := plugin.ApplyRoutePlugin(ctx, rtCtx, child)
		// 		if err != nil {
		// 			contextutils.LoggerFrom(ctx).Errorf("error applying RoutePlugin to child route %s: %v", child.GetName(), err)
		// 		}
		// 	}
		// }

		// Add the delegatee output routes to the final output list
		*outputs = append(*outputs, delegatedRoutes...)

		// It is possible for a parent route to not produce an output route action
		// if it only delegates and does not directly route to a backend.
		// We should only set a direct response action when there is no output action
		// for a parent rule and when there are no delegated routes because this would
		// otherwise result in a top level matcher with a direct response action for the
		// path that the parent is delegating for.
		if len(outputRoute.Backends) == 0 && !delegates {
			// TODO: figure out how to mark this backend with a 500 error

			// outputRoute.Action = &v1.Route_DirectResponseAction{
			// 	DirectResponseAction: &v1.DirectResponseAction{
			// 		Status: http.StatusInternalServerError,
			// 	},
			// }
		}

		// A parent route that delegates to a child route should not have an output route
		// action (outputRoute.Action) as the routes are derived from the child route.
		// So this conditional ensures that we do not create a top level route matcher
		// for the parent route when it delegates to a child route.
		if len(outputRoute.Backends) > 0 {
			routes[idx] = &outputRoute
			// TODO: I think this was redundant/obsolete
			// outputRoute.Matchers = []*matchers.Matcher{translateGlooMatcher(match)}
		}
	}
	return routes
}

func setRouteAction(
	gwroute *RouteInfo,
	rule ir.HttpRouteRuleIR,
	outputRoute *ir.HttpRouteRuleMatchIR,
	match gwv1.HTTPRouteMatch,
	outputs *[]*ir.HttpRouteRuleMatchIR,
	routesVisited sets.Set[types.NamespacedName],
	delegationChain *list.List,
) bool {
	backends := rule.Backends
	delegates := false

	for _, backend := range backends {
		// If the backend is an HTTPRoute, it implies route delegation
		// for which delegated routes are recursively flattened and translated
		if backend.Delegate != nil {
			delegates = true
			// Flatten delegated HTTPRoute references
			err := flattenDelegatedRoutes(
				gwroute,
				backend,
				match,
				outputs,
				routesVisited,
				delegationChain,
			)
			if err != nil {
				// query.ProcessBackendError(err, reporter)
			}
			continue
		}

		httpBackend := ir.HttpBackend{
			Backend:          *backend.Backend, // TODO: Nil check?
			AttachedPolicies: backend.AttachedPolicies,
		}

		// FIXME: handle this stuff
		// for _, bp := range pluginRegistry.GetBackendPlugins() {

		//	var port uint32
		//	if backendRef.Port != nil {
		//		port = uint32(*backendRef.Port)
		//	}

		outputRoute.Backends = append(outputRoute.Backends, httpBackend)
	}

	return delegates
}

func (r *gatewayQueries) GetRoutesForGateway(kctx krt.HandlerContext, ctx context.Context, gw *gwv1.Gateway) (*RoutesForGwResult, error) {
	nns := types.NamespacedName{
		Namespace: gw.Namespace,
		Name:      gw.Name,
	}

	// Process each route
	ret := NewRoutesForGwResult()
	for _, route := range fetchRoutes(kctx, r, nns) {
		if err := r.processRoute(kctx, ctx, gw, route, ret); err != nil {
			return nil, err
		}
	}

	return ret, nil
}

// fetchRoutes is a helper function to fetch routes and add to the routes slice.
func fetchRoutes(kctx krt.HandlerContext, r *gatewayQueries, nns types.NamespacedName) []ir.Route {
	return r.routes.RoutesForGateway(kctx, nns)

}

func (r *gatewayQueries) processRoute(kctx krt.HandlerContext, ctx context.Context, gw *gwv1.Gateway, route ir.Route, ret *RoutesForGwResult) error {
	refs := getParentRefsForGw(gw, route)
	routeKind := route.GetGroupKind().Kind

	for _, ref := range refs {
		anyRoutesAllowed := false
		anyListenerMatched := false
		anyHostsMatch := false

		for _, l := range gw.Spec.Listeners {
			lr := ret.ListenerResults[string(l.Name)]
			if lr == nil {
				lr = &ListenerResult{}
				ret.ListenerResults[string(l.Name)] = lr
			}

			allowedNs, allowedKinds, err := r.allowedRoutes(gw, &l)
			if err != nil {
				lr.Error = err
				continue
			}

			// Check if the kind of the route is allowed by the listener
			if !isKindAllowed(routeKind, allowedKinds) {
				continue
			}

			// Check if the namespace of the route is allowed by the listener
			if !allowedNs(route.GetNamespace()) {
				continue
			}
			anyRoutesAllowed = true

			// Check if the listener matches the route's parent reference
			if !parentRefMatchListener(&ref, &l) {
				continue
			}
			anyListenerMatched = true

			// If the route is an HTTPRoute, check the hostname intersection
			var hostnames []string
			if routeKind == wellknown.HTTPRouteKind {
				if hr, ok := route.(*ir.HttpRouteIR); ok {
					var ok bool
					ok, hostnames = hostnameIntersect(&l, hr.Hostnames)
					if !ok {
						continue
					}
					anyHostsMatch = true
				}
			}

			// If all checks pass, add the route to the listener result
			lr.Routes = append(lr.Routes, r.GetRouteChain(kctx, ctx, route, hostnames, ref))
		}

		// Handle route errors based on checks
		if !anyRoutesAllowed {
			ret.RouteErrors = append(ret.RouteErrors, &RouteError{
				Route:     route.GetSourceObject(),
				ParentRef: ref,
				Error:     Error{E: ErrNotAllowedByListeners, Reason: gwv1.RouteReasonNotAllowedByListeners},
			})
		} else if !anyListenerMatched {
			ret.RouteErrors = append(ret.RouteErrors, &RouteError{
				Route:     route.GetSourceObject(),
				ParentRef: ref,
				Error:     Error{E: ErrNoMatchingParent, Reason: gwv1.RouteReasonNoMatchingParent},
			})
		} else if routeKind == wellknown.HTTPRouteKind && !anyHostsMatch {
			ret.RouteErrors = append(ret.RouteErrors, &RouteError{
				Route:     route.GetSourceObject(),
				ParentRef: ref,
				Error:     Error{E: ErrNoMatchingListenerHostname, Reason: gwv1.RouteReasonNoMatchingListenerHostname},
			})
		}
	}

	return nil
}

// isKindAllowed is a helper function to check if a kind is allowed.
func isKindAllowed(routeKind string, allowedKinds []metav1.GroupKind) bool {
	for _, kind := range allowedKinds {
		if kind.Kind == routeKind {
			return true
		}
	}
	return false
}

type Namespaced interface {
	GetName() string
	GetNamespace() string
}

func namespacedName(o Namespaced) types.NamespacedName {
	return types.NamespacedName{Name: o.GetName(), Namespace: o.GetNamespace()}
}

// getRouteItems extracts the list of route items from the provided client.ObjectList.
// Supported route list types are:
//
//   - HTTPRouteList
//   - TCPRouteList
func getRouteItems(list client.ObjectList) ([]client.Object, error) {
	switch routes := list.(type) {
	case *gwv1.HTTPRouteList:
		var objs []client.Object
		for i := range routes.Items {
			objs = append(objs, &routes.Items[i])
		}
		return objs, nil
	case *gwv1a2.TCPRouteList:
		var objs []client.Object
		for i := range routes.Items {
			objs = append(objs, &routes.Items[i])
		}
		return objs, nil
	default:
		return nil, fmt.Errorf("unsupported route type %T", list)
	}
}
