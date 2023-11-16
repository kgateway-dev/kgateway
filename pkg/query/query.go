package query

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	ErrMissingReferenceGrant      = fmt.Errorf("missing reference grant")
	ErrUnknownKind                = fmt.Errorf("unknown kind")
	ErrNoMatchingListenerHostname = fmt.Errorf("no matching listener hostname")
	ErrNoMatchingParent           = fmt.Errorf("no matching parent")
	ErrNotAllowedByListeners      = fmt.Errorf("not allowed by listeners")
)

type Error struct {
	Reason apiv1.RouteConditionReason
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

type FromGkNs struct {
	Gk metav1.GroupKind
	Ns string
}

func (f FromGkNs) GroupKind() (metav1.GroupKind, error) {
	return f.Gk, nil
}

func (f FromGkNs) Namespace() string {
	return f.Ns
}

type GatewayQueries interface {
	ObjToFrom(obj client.Object) From

	// Returns map of listener names -> list of http routes.
	GetRoutesForGw(ctx context.Context, gw *apiv1.Gateway) (RoutesForGwResult, error)
	// Given a backendRef that resides in namespace obj, return the service that backs it.
	// This will error with `ErrMissingReferenceGrant` if there is no reference grant allowing the reference
	// return value depends on the group/kind in the backendRef.
	GetBackendForRef(ctx context.Context, obj From, backendRef *apiv1.BackendObjectReference) (client.Object, error)

	GetSecretForRef(ctx context.Context, obj From, secretRef apiv1.SecretObjectReference) (client.Object, error)
}

type RoutesForGwResult struct {
	// key is listener name
	ListenerResults map[string]*ListenerResult
	RouteErrors     []*RouteError
}

type ListenerResult struct {
	Error  error
	Routes []*ListenerRouteResult
}

type ListenerRouteResult struct {
	Route     apiv1.HTTPRoute
	ParentRef apiv1.ParentReference
	Hostnames []string
}

type RouteError struct {
	Route     apiv1.HTTPRoute
	ParentRef apiv1.ParentReference
	Error     Error
}

func NewData(c client.Client, scheme *runtime.Scheme) GatewayQueries {
	return &gatewayQueries{c, scheme}
}

type gatewayQueries struct {
	client client.Client
	scheme *runtime.Scheme
}

func (r *gatewayQueries) referenceAllowed(ctx context.Context, from metav1.GroupKind, fromns string, to metav1.GroupKind, tons, toname string) (bool, error) {

	var list apiv1beta1.ReferenceGrantList
	err := r.client.List(ctx, &list, client.InNamespace(tons), client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(ReferenceGrantFromField, fromns)})
	if err != nil {
		return false, err
	}

	return ReferenceAllowed(ctx, from, fromns, to, toname, list.Items), nil
}

func (r *gatewayQueries) ObjToFrom(obj client.Object) From {
	return FromObject{Object: obj, Scheme: r.scheme}
}

func (r *gatewayQueries) GetRoutesForGw(ctx context.Context, gw *apiv1.Gateway) (RoutesForGwResult, error) {
	ret := RoutesForGwResult{
		ListenerResults: map[string]*ListenerResult{},
	}

	nns := types.NamespacedName{
		Namespace: gw.Namespace,
		Name:      gw.Name,
	}

	var hrlist apiv1.HTTPRouteList
	err := r.client.List(ctx, &hrlist, client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(HttpRouteTargetField, nns.String())})
	if err != nil {
		return ret, err
	}

	for _, hr := range hrlist.Items {
		refs := getParentRefsForGw(gw, &hr)
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

				if isHttpRouteAllowed(allowedKinds) {
					if !allowedNs(hr.Namespace) {
						continue
					}
					anyRoutesAllowed = true

					if !parentRefMatchListener(ref, &l) {
						continue
					}
					anyListenerMatched = true
					if ok, hostnames := hostnameIntersect(&l, &hr); ok {
						lrr := &ListenerRouteResult{
							Route:     hr,
							Hostnames: hostnames,
							ParentRef: ref,
						}
						anyHostsMatch = true
						lr.Routes = append(lr.Routes, lrr)
					}
				}
			}

			if !anyRoutesAllowed {
				ret.RouteErrors = append(ret.RouteErrors, &RouteError{
					Route:     hr,
					ParentRef: ref,
					Error:     Error{E: ErrNotAllowedByListeners, Reason: apiv1.RouteReasonNotAllowedByListeners},
				})
			} else if !anyListenerMatched {
				ret.RouteErrors = append(ret.RouteErrors, &RouteError{
					Route:     hr,
					ParentRef: ref,
					Error:     Error{E: ErrNoMatchingParent, Reason: apiv1.RouteReasonNoMatchingParent},
				})
			} else if !anyHostsMatch {
				ret.RouteErrors = append(ret.RouteErrors, &RouteError{
					Route:     hr,
					ParentRef: ref,
					Error:     Error{E: ErrNoMatchingListenerHostname, Reason: apiv1.RouteReasonNoMatchingListenerHostname},
				})
			}
		}
	}
	return ret, nil
}

func (r *gatewayQueries) allowedRoutes(gw *apiv1.Gateway, l *apiv1.Listener) (func(string) bool, []metav1.GroupKind, error) {
	var allowedKinds []metav1.GroupKind

	switch l.Protocol {
	case apiv1.HTTPSProtocolType:
		fallthrough
	case apiv1.HTTPProtocolType:
		allowedKinds = []metav1.GroupKind{{Kind: "HTTPRoute", Group: "gateway.networking.k8s.io"}}
	case apiv1.TLSProtocolType:
		fallthrough
	case apiv1.TCPProtocolType:
		allowedKinds = []metav1.GroupKind{{}}
	case apiv1.UDPProtocolType:
		allowedKinds = []metav1.GroupKind{{}}
	}

	allowedNs := SameNamespace(gw.Namespace)
	if ar := l.AllowedRoutes; ar != nil {
		if ar.Kinds != nil {
			allowedKinds = nil
			for _, k := range ar.Kinds {
				gk := metav1.GroupKind{Kind: string(k.Kind)}
				if k.Group != nil {
					gk.Group = string(*k.Group)
				} else {
					gk.Group = "gateway.networking.k8s.io"
				}
				allowedKinds = append(allowedKinds, gk)
			}
		}
		if ar.Namespaces != nil && ar.Namespaces.From != nil {
			switch *ar.Namespaces.From {
			case apiv1.NamespacesFromAll:
				allowedNs = AllNamespace()
			case apiv1.NamespacesFromSelector:
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

func parentRefMatchListener(ref apiv1.ParentReference, l *apiv1.Listener) bool {
	if ref.Port != nil && *ref.Port != l.Port {
		return false
	}
	if ref.SectionName != nil && *ref.SectionName != l.Name {
		return false
	}
	return true
}

func getParentRefsForGw(gw *apiv1.Gateway, hr *apiv1.HTTPRoute) []apiv1.ParentReference {
	var ret []apiv1.ParentReference
	for _, pRef := range hr.Spec.ParentRefs {

		if pRef.Group != nil && *pRef.Group != "gateway.networking.k8s.io" {
			continue
		}
		if pRef.Kind != nil && *pRef.Kind != "Gateway" {
			continue
		}
		ns := hr.Namespace
		if pRef.Namespace != nil {
			ns = string(*pRef.Namespace)
		}

		if ns == gw.Namespace && string(pRef.Name) == gw.Name {
			ret = append(ret, pRef)
		}
	}
	return ret
}

func hostnameIntersect(l *apiv1.Listener, hr *apiv1.HTTPRoute) (bool, []string) {
	var hostnames []string
	if l.Hostname == nil {
		for _, h := range hr.Spec.Hostnames {
			hostnames = append(hostnames, string(h))
		}
		return true, hostnames
	}
	var listenerHostname string = string(*l.Hostname)

	if strings.HasPrefix(listenerHostname, "*.") {
		if hr.Spec.Hostnames == nil {
			return true, []string{listenerHostname}
		}

		for _, hostname := range hr.Spec.Hostnames {
			hrHost := string(hostname)
			if strings.HasSuffix(hrHost, listenerHostname[1:]) {
				hostnames = append(hostnames, hrHost)
			}
		}
		return len(hostnames) > 0, hostnames
	} else {
		if len(hr.Spec.Hostnames) == 0 {
			return true, []string{listenerHostname}
		}
		for _, hostname := range hr.Spec.Hostnames {
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
	}

	return false, nil
}

func (r *gatewayQueries) GetSecretForRef(ctx context.Context, obj From, secretRef apiv1.SecretObjectReference) (client.Object, error) {
	secretKind := "Secret"
	secretGroup := ""

	if secretRef.Group != nil {
		secretGroup = string(*secretRef.Group)
	}
	if secretRef.Kind != nil {
		secretKind = string(*secretRef.Kind)
	}
	secretGK := metav1.GroupKind{Group: secretGroup, Kind: secretKind}

	return r.getRef(ctx, obj, string(secretRef.Name), secretRef.Namespace, secretGK)
}

func (r *gatewayQueries) GetBackendForRef(ctx context.Context, obj From, backend *apiv1.BackendObjectReference) (client.Object, error) {
	backendKind := "Service"
	backendGroup := ""

	if backend.Group != nil {
		backendGroup = string(*backend.Group)
	}
	if backend.Kind != nil {
		backendKind = string(*backend.Kind)
	}
	backendGK := metav1.GroupKind{Group: backendGroup, Kind: backendKind}

	return r.getRef(ctx, obj, string(backend.Name), backend.Namespace, backendGK)
}

func (r *gatewayQueries) getRef(ctx context.Context, from From, backendName string, backendNS *apiv1.Namespace, backendGK metav1.GroupKind) (client.Object, error) {

	ns := from.Namespace()
	if backendNS != nil {
		ns = string(*backendNS)
	}
	if ns != from.Namespace() {

		fromgk, err := from.GroupKind()
		if err != nil {
			return nil, err
		}
		// check if we're allowed to reference this namespace
		allowed, err := r.referenceAllowed(ctx, fromgk, from.Namespace(), backendGK, ns, backendName)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrMissingReferenceGrant
		}
	}

	gk := schema.GroupKind{Group: backendGK.Group, Kind: backendGK.Kind}

	versions := r.scheme.VersionsForGroupKind(gk)
	// versions are prioritized by order in the scheme, so we can just take the first one
	if len(versions) == 0 {
		return nil, ErrUnknownKind
	}
	newObj, err := r.scheme.New(gk.WithVersion(versions[0].Version))
	if err != nil {
		return nil, err
	}
	ret, ok := newObj.(client.Object)
	if !ok {
		return nil, fmt.Errorf("new object is not a client.Object")
	}

	err = r.client.Get(ctx, types.NamespacedName{Namespace: ns, Name: backendName}, ret)
	if err != nil {
		return nil, err
	}
	return ret, nil

}

func isHttpRouteAllowed(allowedKinds []metav1.GroupKind) bool {
	return isRouteAllowed("gateway.networking.k8s.io", "HTTPRoute", allowedKinds)
}

func isRouteAllowed(group, kind string, allowedKinds []metav1.GroupKind) bool {
	for _, k := range allowedKinds {
		var allowedGroup string = k.Group
		if allowedGroup == "" {
			allowedGroup = "gateway.networking.k8s.io"
		}

		if allowedGroup == group && k.Kind == kind {
			return true
		}
	}
	return false
}

func SameNamespace(ns string) func(string) bool {
	return func(s string) bool {
		return ns == s
	}
}

func AllNamespace() func(string) bool {
	return func(s string) bool {
		return true
	}
}

func (r *gatewayQueries) NamespaceSelector(sel labels.Selector) func(string) bool {
	return func(s string) bool {
		var ns corev1.Namespace
		r.client.Get(context.TODO(), types.NamespacedName{Name: s}, &ns)
		return sel.Matches(labels.Set(ns.Labels))
	}
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
