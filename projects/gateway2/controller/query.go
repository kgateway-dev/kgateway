package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	ErrMissingReferenceGrant = fmt.Errorf("missing reference grant")
)

type GatewayQueries interface {
	// Returns map of listener names -> list of http routes.
	GetRoutesForGw(ctx context.Context, gw *api.Gateway) (map[string][]api.HTTPRoute, error)
	// Given a backendRef that resides in namespace routeNs, return the service that backs it.
	// This will error with `ErrMissingReferenceGrant` if there is no reference grant allowing the reference
	// return value depends on the group/kind in the backendRef.
	GetBackendsForRef(ctx context.Context, routeNs string, backendRef *api.HTTPBackendRef) (client.Object, error)

	GetSecretForRef(ctx context.Context, secretRefNs string, secretRef *api.SecretObjectReference) (client.Object, error)
}

func NewData(c client.Client, scheme *runtime.Scheme) GatewayQueries {
	return &gatewayQueries{c, scheme}
}

type gatewayQueries struct {
	client client.Client
	scheme *runtime.Scheme
}

func (r *gatewayQueries) referenceAllowed(ctx context.Context, from metav1.GroupKind, fromns string, to metav1.GroupKind, tons, toname string) (bool, error) {

	var list api.ReferenceGrantList
	err := r.client.List(ctx, &list, client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(referenceGrantFrom, tons)})
	if err != nil {
		return false, err
	}
	return ReferenceAllowed(ctx, from, fromns, to, toname, list.Items), nil
}

func (r *gatewayQueries) GetRoutesForGw(ctx context.Context, gw *api.Gateway) (map[string][]api.HTTPRoute, error) {
	nns := types.NamespacedName{
		Namespace: gw.Namespace,
		Name:      gw.Name,
	}

	var hrlist api.HTTPRouteList
	err := r.client.List(ctx, &hrlist, client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(httpRouteTargetField, nns.String())})
	if err != nil {
		return nil, err
	}
	ret := map[string][]api.HTTPRoute{}
	for _, l := range gw.Spec.Listeners {
		if _, ok := ret[string(l.Name)]; ok {
			return nil, fmt.Errorf("duplicate listener name %s", l.Name)
		}

		var allowedKinds []metav1.GroupKind

		switch l.Protocol {
		case api.HTTPSProtocolType:
			fallthrough
		case api.HTTPProtocolType:
			allowedKinds = []metav1.GroupKind{{Kind: string(kind(&api.HTTPRoute{})), Group: "gateway.networking.k8s.io"}}
		case api.TLSProtocolType:
			fallthrough
		case api.TCPProtocolType:
			allowedKinds = []metav1.GroupKind{{}}
		case api.UDPProtocolType:
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
					}
					allowedKinds = append(allowedKinds, gk)
				}
			}
			if ar.Namespaces != nil && ar.Namespaces.From != nil {
				switch *ar.Namespaces.From {
				case api.NamespacesFromAll:
					allowedNs = AllNamespace()
				case api.NamespacesFromSelector:
					if ar.Namespaces.Selector == nil {
						return nil, fmt.Errorf("selector must be set")
					}
					selector, err := metav1.LabelSelectorAsSelector(ar.Namespaces.Selector)
					if err != nil {
						return nil, err
					}
					allowedNs = r.NamespaceSelector(selector)
				}

			}
		}
		if isHttpRouteAllowed(allowedKinds) {
			var result []api.HTTPRoute
			for _, hr := range hrlist.Items {
				if !allowedNs(hr.Namespace) {
					continue
				}

				// TODO: more checks - verify hostname matches.
				result = append(result, hr)
			}
			ret[string(l.Name)] = result
		}
	}
	return ret, err
}

func (r *gatewayQueries) GetSecretForRef(ctx context.Context, secretRefNs string, secretRef *api.SecretObjectReference) (client.Object, error) {
	secretKind := "Secret"
	secretGroup := ""

	if secretRef.Group != nil {
		secretGroup = string(*secretRef.Group)
	}
	if secretRef.Kind != nil {
		secretKind = string(*secretRef.Kind)
	}
	from := metav1.GroupKind{Group: "networking.gateway.k8s.io", Kind: "Gateway"}
	secretGK := metav1.GroupKind{Group: secretGroup, Kind: secretKind}

	return r.getRef(ctx, from, secretRefNs, string(secretRef.Name), secretRef.Namespace, secretGK)
}

func (r *gatewayQueries) GetBackendsForRef(ctx context.Context, routeNs string, backend *api.HTTPBackendRef) (client.Object, error) {
	backendKind := "Service"
	backendGroup := ""

	if backend.Group != nil {
		backendGroup = string(*backend.Group)
	}
	if backend.Kind != nil {
		backendKind = string(*backend.Kind)
	}
	from := metav1.GroupKind{Group: "networking.gateway.k8s.io", Kind: "HTTPRoute"}
	backendGK := metav1.GroupKind{Group: backendGroup, Kind: backendKind}

	return r.getRef(ctx, from, routeNs, string(backend.Name), backend.Namespace, backendGK)
}

func (r *gatewayQueries) getRef(ctx context.Context, fromgk metav1.GroupKind, fromNs string, backendName string, backendNS *api.Namespace, backendGK metav1.GroupKind) (client.Object, error) {

	ns := fromNs
	if backendNS != nil {
		ns = string(*backendNS)
	}

	if ns != fromNs {
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
		return nil, fmt.Errorf("no versions for group kind %v", gk)
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
	return ret, err

}

func isHttpRouteAllowed(allowedKinds []metav1.GroupKind) bool {

	for _, k := range allowedKinds {
		if (k.Group != "" || k.Group == "gateway.networking.k8s.io") && k.Kind == string(kind(&api.HTTPRoute{})) {
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

func ReferenceAllowed(ctx context.Context, fromgk metav1.GroupKind, fromns string, togk metav1.GroupKind, toname string, grantsInToNs []api.ReferenceGrant) bool {
	for _, refGrant := range grantsInToNs {
		for _, from := range refGrant.Spec.From {
			if fromgk.Group == string(from.Group) && fromgk.Kind == string(from.Kind) {
				for _, to := range refGrant.Spec.To {
					if togk.Group == string(to.Group) && togk.Kind == string(to.Kind) {
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
