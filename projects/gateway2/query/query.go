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
	apiv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/solo-io/gloo/projects/gateway2/wellknown"
)

var (
	ErrMissingReferenceGrant      = fmt.Errorf("missing reference grant")
	ErrUnknownBackendKind         = fmt.Errorf("unknown backend kind")
	ErrNoMatchingListenerHostname = fmt.Errorf("no matching listener hostname")
	ErrNoMatchingParent           = fmt.Errorf("no matching parent")
	ErrNotAllowedByListeners      = fmt.Errorf("not allowed by listeners")
	ErrLocalObjRefMissingKind     = fmt.Errorf("localObjRef provided with empty kind")
	ErrCyclicReference            = fmt.Errorf("cyclic reference detected while evaluating delegated routes")
	ErrUnresolvedReference        = fmt.Errorf("unresolved reference")
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

// TODO(Law): remove this type entirely?
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

	// Given a backendRef that resides in namespace obj, return the service that backs it.
	// This will error with `ErrMissingReferenceGrant` if there is no reference grant allowing the reference
	// return value depends on the group/kind in the backendRef.
	GetBackendForRef(ctx context.Context, obj From, backendRef *apiv1.BackendObjectReference) (client.Object, error)

	GetSecretForRef(ctx context.Context, obj From, secretRef apiv1.SecretObjectReference) (client.Object, error)

	GetLocalObjRef(ctx context.Context, from From, localObjRef apiv1.LocalObjectReference) (client.Object, error)

	// GetRoutesForGateway finds the top level xRoutes attached to the provided Gateway
	GetRoutesForGateway(ctx context.Context, gw *apiv1.Gateway) (*RoutesForGwResult, error)
	// GetRouteChain resolves backends and delegated routes for a the provided xRoute object
	GetRouteChain(ctx context.Context, obj client.Object, hostnames []string, parentRef apiv1.ParentReference) *RouteInfo
}

type RoutesForGwResult struct {
	// key is listener name
	ListenerResults map[string]*ListenerResult
	RouteErrors     []*RouteError
}

type ListenerResult struct {
	Error  error
	Routes []*RouteInfo
}

type RouteError struct {
	Route     client.Object
	ParentRef apiv1.ParentReference
	Error     Error
}

type options struct {
	customBackendResolvers []BackendRefResolver
}

type Option func(*options)

func WithBackendRefResolvers(
	customBackendResolvers ...BackendRefResolver,
) Option {
	return func(o *options) {
		o.customBackendResolvers = append(o.customBackendResolvers, customBackendResolvers...)
	}
}

func NewData(
	c client.Client,
	scheme *runtime.Scheme,
	reqCRDsExist *bool,
	opts ...Option,
) GatewayQueries {
	builtOpts := &options{}
	for _, opt := range opts {
		opt(builtOpts)
	}
	return &gatewayQueries{
		client:                 c,
		scheme:                 scheme,
		requiredCRDsExist:      reqCRDsExist,
		customBackendResolvers: builtOpts.customBackendResolvers,
	}
}

// NewRoutesForGwResult creates and returns a new RoutesForGwResult with initialized fields.
func NewRoutesForGwResult() *RoutesForGwResult {
	return &RoutesForGwResult{
		ListenerResults: make(map[string]*ListenerResult),
		RouteErrors:     []*RouteError{},
	}
}

type gatewayQueries struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	customBackendResolvers []BackendRefResolver
	// Cache whether the required Gateway API CRDs are installed.
	requiredCRDsExist *bool
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

func parentRefMatchListener(ref *apiv1.ParentReference, l *apiv1.Listener) bool {
	if ref != nil && ref.Port != nil && *ref.Port != l.Port {
		return false
	}
	if ref.SectionName != nil && *ref.SectionName != l.Name {
		return false
	}
	return true
}

// getParentRefsForGw extracts the ParentReferences from the provided object for the provided Gateway.
// Supported object types are:
//
//   - HTTPRoute
//   - TCPRoute
func getParentRefsForGw(gw *apiv1.Gateway, obj client.Object) []apiv1.ParentReference {
	var ret []apiv1.ParentReference

	switch route := obj.(type) {
	case *apiv1.HTTPRoute:
		for _, pRef := range route.Spec.ParentRefs {
			if isParentRefForGw(&pRef, gw, route.Namespace) {
				ret = append(ret, pRef)
			}
		}
	case *apiv1alpha2.TCPRoute:
		for _, pRef := range route.Spec.ParentRefs {
			if isParentRefForGw(&pRef, gw, route.Namespace) {
				ret = append(ret, pRef)
			}
		}
	default:
		// Unsupported route type
		// TODO (danehans): Should we should capture this as a metric?
		return ret
	}

	return ret
}

// isParentRefForGw checks if a ParentReference is associated with the provided Gateway.
func isParentRefForGw(pRef *apiv1.ParentReference, gw *apiv1.Gateway, defaultNs string) bool {
	if gw == nil || pRef == nil {
		return false
	}

	if pRef.Group != nil && *pRef.Group != apiv1.GroupName {
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

func hostnameIntersect(l *apiv1.Listener, hr *apiv1.HTTPRoute) (bool, []string) {
	var hostnames []string
	if l == nil || hr == nil {
		return false, hostnames
	}
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

func (r *gatewayQueries) GetLocalObjRef(ctx context.Context, obj From, localObjRef apiv1.LocalObjectReference) (client.Object, error) {
	refGroup := ""
	if localObjRef.Group != "" {
		refGroup = string(localObjRef.Group)
	}

	if localObjRef.Kind == "" {
		return nil, ErrLocalObjRefMissingKind
	}
	refKind := localObjRef.Kind

	localObjGK := metav1.GroupKind{Group: refGroup, Kind: string(refKind)}
	return r.getRef(ctx, obj, string(localObjRef.Name), nil, localObjGK)
}

func (r *gatewayQueries) GetBackendForRef(ctx context.Context, obj From, backend *apiv1.BackendObjectReference) (client.Object, error) {
	for _, cr := range r.customBackendResolvers {
		if o, err, ok := cr.GetBackendForRef(ctx, obj, backend); ok {
			return o, err
		}
	}

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

// BackendRefResolver allows resolution of backendRefs with a custom format.
type BackendRefResolver interface {
	// GetBackendForRef resolves a custom reference. When the bool return is false,
	// indicates that the resolver is not responsible for the given ref.
	GetBackendForRef(ctx context.Context, obj From, backend *apiv1.BackendObjectReference) (client.Object, error, bool)
}

func (r *gatewayQueries) getRef(ctx context.Context, from From, backendName string, backendNS *apiv1.Namespace, backendGK metav1.GroupKind) (client.Object, error) {
	fromNs := from.Namespace()
	if fromNs == "" {
		fromNs = "default"
	}
	ns := fromNs
	if backendNS != nil {
		ns = string(*backendNS)
	}
	if ns != fromNs {
		fromgk, err := from.GroupKind()
		if err != nil {
			return nil, err
		}
		// check if we're allowed to reference this namespace
		allowed, err := r.referenceAllowed(ctx, fromgk, fromNs, backendGK, ns, backendName)
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
		return nil, ErrUnknownBackendKind
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
