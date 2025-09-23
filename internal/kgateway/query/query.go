package query

import (
	"context"
	"fmt"
	"strings"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	apixv1alpha1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

var (
	ErrNoMatchingListenerHostname = fmt.Errorf("no matching listener hostname")
	ErrNoMatchingParent           = fmt.Errorf("no matching parent")
	ErrNotAllowedByListeners      = fmt.Errorf("not allowed by listeners")
	ErrLocalObjRefMissingKind     = fmt.Errorf("localObjRef provided with empty kind")
	ErrCyclicReference            = fmt.Errorf("cyclic reference detected while evaluating delegated routes")
	ErrUnresolvedReference        = fmt.Errorf("unresolved reference")
)

type Error struct {
	Reason gwv1.RouteConditionReason
	E      error
}

var _ error = &Error{}

// Error implements error.
func (e *Error) Error() string {
	return string(e.Reason)
}

func (e *Error) Unwrap() error {
	return e.E
}

type GroupKindNs struct {
	gk metav1.GroupKind
	ns string
}

func (g *GroupKindNs) GroupKind() (metav1.GroupKind, error) {
	return g.gk, nil
}

func (g *GroupKindNs) Namespace() string {
	return g.ns
}

func NewGroupKindNs(gk metav1.GroupKind, ns string) *GroupKindNs {
	return &GroupKindNs{
		gk: gk,
		ns: ns,
	}
}

type From interface {
	GroupKind() (metav1.GroupKind, error)
	Namespace() string
}

type FromObject struct {
	client.Object
	Scheme *runtime.Scheme
}

func (f FromObject) GroupKind() (metav1.GroupKind, error) {
	scheme := f.Scheme
	from := f.Object
	gvks, isUnversioned, err := scheme.ObjectKinds(from)
	var zero metav1.GroupKind
	if err != nil {
		return zero, fmt.Errorf("failed to get object kind %T", from)
	}
	if isUnversioned {
		return zero, fmt.Errorf("object of type %T is not versioned", from)
	}
	if len(gvks) != 1 {
		return zero, fmt.Errorf("ambigous gvks for %T, %v", f, gvks)
	}
	gvk := gvks[0]
	return metav1.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, nil
}

func (f FromObject) Namespace() string {
	return f.GetNamespace()
}

type GatewayQueries interface {
	GetSecretForRef(kctx krt.HandlerContext, ctx context.Context, fromGk schema.GroupKind, fromns string, secretRef gwv1.SecretObjectReference) (*ir.Secret, error)

	// GetRoutesForGateway finds the top level xRoutes attached to the provided Gateway
	GetRoutesForGateway(kctx krt.HandlerContext, ctx context.Context, gw *ir.Gateway) (*RoutesForGwResult, error)
	// GetRouteChain resolves backends and delegated routes for a the provided xRoute object
	GetRouteChain(kctx krt.HandlerContext,
		ctx context.Context,
		route ir.Route,
		hostnames []string,
		parentRef gwv1.ParentReference,
	) *RouteInfo
}

type RoutesForGwResult struct {
	// key is <parent.Namespace/parent.Name/listener.Name>
	listenerResults map[string]*ListenerResult
	RouteErrors     []*RouteError
}

func (r *RoutesForGwResult) GetListenerResult(parent client.Object, listenerName string) *ListenerResult {
	return r.listenerResults[GenerateRouteKey(parent, listenerName)]
}

func (r *RoutesForGwResult) GetListenerResults(yield func(string, *ListenerResult) bool) {
	for k, v := range r.listenerResults {
		if !yield(k, v) {
			return
		}
	}
}

func (r *RoutesForGwResult) setListenerResult(parent client.Object, listenerName string, result *ListenerResult) {
	r.listenerResults[GenerateRouteKey(parent, listenerName)] = result
}

func (r *RoutesForGwResult) merge(r2 *RoutesForGwResult) {
	for k, v := range r2.listenerResults {
		r.listenerResults[k] = v
	}
	r.RouteErrors = append(r.RouteErrors, r2.RouteErrors...)
}

type ListenerResult struct {
	Error  error
	Routes []*RouteInfo
}

type RouteError struct {
	Route     ir.Route
	ParentRef gwv1.ParentReference
	Error     Error
}

// NewData wraps a _pointer_ to CommonCollections. We take a reference because
// the queries aren't ready until InitPlugins has been called on the
// CommonCollections.
func NewData(
	collections *collections.CommonCollections,
) GatewayQueries {
	return &gatewayQueries{
		collections: collections,
	}
}

// NewRoutesForGwResult creates and returns a new RoutesForGwResult with initialized fields.
func NewRoutesForGwResult() *RoutesForGwResult {
	return &RoutesForGwResult{
		listenerResults: make(map[string]*ListenerResult),
		RouteErrors:     []*RouteError{},
	}
}

type gatewayQueries struct {
	collections *collections.CommonCollections
}

func parentRefMatchListener(ref *gwv1.ParentReference, l *gwv1.Listener) bool {
	if ref != nil && ref.Port != nil && *ref.Port != l.Port {
		return false
	}
	if ref.SectionName != nil && *ref.SectionName != l.Name {
		return false
	}
	return true
}

// getParentRefsForResource extracts the ParentReferences from the provided object for the provided Gateway.
// Supported object types are:
//
//   - HTTPRoute
//   - TCPRoute
//   - TLSRoute
//   - GRPCRoute
func getParentRefsForResource(resource client.Object, obj ir.Route) []gwv1.ParentReference {
	var ret []gwv1.ParentReference

	for _, pRef := range obj.GetParentRefs() {
		if isParentRefForResource(&pRef, resource, obj.GetNamespace()) {
			ret = append(ret, pRef)
		}
	}

	return ret
}

// isParentRefForResource checks if a ParentReference is associated with the provided resource.
// The resource must either be a Gateway or a ListenerSet
func isParentRefForResource(pRef *gwv1.ParentReference, resource client.Object, defaultNs string) bool {
	if resource == nil || pRef == nil {
		return false
	}

	switch typed := resource.(type) {
	case *gwv1.Gateway:
		return isParentRefForGw(pRef, typed, defaultNs)
	case *apixv1alpha1.XListenerSet:
		// Check if the route belongs to the parent gateway. If so accept it
		parentRef := getParentGatewayRef(typed)
		parentGW := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: parentRef.Namespace,
				Name:      parentRef.Name,
			},
		}
		gatewayRoute := isParentRefForGw(pRef, parentGW, defaultNs)

		if gatewayRoute {
			// If it is attached to the gateway but has a section name, it won't attach to the listener set
			if pRef.SectionName != nil && *pRef.SectionName != "" {
				return false
			}
			// If it is attached to the gateway but has no section name, it applies to the listener set also
			return true
		} else {
			// Is it attached to a listener set and not a gateway ?
			if pRef.Group != nil && *pRef.Group != apixv1alpha1.GroupName {
				return false
			}
			if pRef.Kind != nil && *pRef.Kind != wellknown.XListenerSetKind {
				return false
			}

			ns := defaultNs
			if pRef.Namespace != nil {
				ns = string(*pRef.Namespace)
			}
			// Does it attach to this resource ?
			return ns == typed.Namespace && string(pRef.Name) == typed.Name
		}
	default:
		return false
	}
}

// isParentRefForGw checks if a ParentReference is associated with the provided Gateway.
func isParentRefForGw(pRef *gwv1.ParentReference, gw *gwv1.Gateway, defaultNs string) bool {
	if gw == nil || pRef == nil {
		return false
	}

	if pRef.Group != nil && *pRef.Group != gwv1.GroupName {
		return false
	}
	if pRef.Kind != nil && *pRef.Kind != wellknown.GatewayKind {
		return false
	}

	ns := defaultNs
	if pRef.Namespace != nil {
		ns = string(*pRef.Namespace)
	}

	return ns == gw.Namespace && string(pRef.Name) == gw.Name
}

func hostnameIntersect(l *gwv1.Listener, routeHostnames []string) (bool, []string) {
	var hostnames []string
	if l == nil {
		return false, hostnames
	}
	if l.Hostname == nil {
		for _, h := range routeHostnames {
			hostnames = append(hostnames, string(h))
		}
		return true, hostnames
	}
	var listenerHostname string = string(*l.Hostname)

	if strings.HasPrefix(listenerHostname, "*.") {
		if len(routeHostnames) == 0 {
			return true, []string{listenerHostname}
		}

		for _, hostname := range routeHostnames {
			hrHost := string(hostname)
			if strings.HasSuffix(hrHost, listenerHostname[1:]) {
				hostnames = append(hostnames, hrHost)
			}
		}
		return len(hostnames) > 0, hostnames
	}
	if len(routeHostnames) == 0 {
		return true, []string{listenerHostname}
	}
	for _, hostname := range routeHostnames {
		hrHost := string(hostname)
		if hrHost == listenerHostname {
			return true, []string{listenerHostname}
		}

		if strings.HasPrefix(hrHost, "*.") {
			if strings.HasSuffix(listenerHostname, hrHost[1:]) {
				return true, []string{listenerHostname}
			}
		}
		// also possible that listener hostname is more specific than the hr hostname
	}

	return false, nil
}

func (r *gatewayQueries) GetSecretForRef(kctx krt.HandlerContext, ctx context.Context, fromGk schema.GroupKind, fromns string, secretRef gwv1.SecretObjectReference) (*ir.Secret, error) {
	f := krtcollections.From{
		GroupKind: fromGk,
		Namespace: fromns,
	}
	return r.collections.Secrets.GetSecret(kctx, f, secretRef)
}

func ReferenceAllowed(ctx context.Context, fromgk metav1.GroupKind, fromns string, togk metav1.GroupKind, toname string, grantsInToNs []apiv1beta1.ReferenceGrant) bool {
	for _, refGrant := range grantsInToNs {
		for _, from := range refGrant.Spec.From {
			if string(from.Namespace) != fromns {
				continue
			}
			if coreIfEmpty(fromgk.Group) == coreIfEmpty(string(from.Group)) && fromgk.Kind == string(from.Kind) {
				for _, to := range refGrant.Spec.To {
					if coreIfEmpty(togk.Group) == coreIfEmpty(string(to.Group)) && togk.Kind == string(to.Kind) {
						if to.Name == nil || string(*to.Name) == toname {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// Note that the spec has examples where the "core" api group is explicitly specified.
// so this helper function converts an empty string (which implies core api group) to the
// explicit "core" api group. It should only be used in places where the spec specifies that empty
// group means "core" api group (some place in the spec may default to the "gateway" api group instead.
func coreIfEmpty(s string) string {
	if s == "" {
		return "core"
	}
	return s
}
