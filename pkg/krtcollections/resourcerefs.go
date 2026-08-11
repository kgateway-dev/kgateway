package krtcollections

import (
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/ondemand"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// GatewayResourceRefs declares the Secrets that core Gateway API translation
// reads: listener serving certificates and the Gateway's backend client
// certificate.
//
// It derives from the *raw* Gateway and ListenerSet collections on purpose.
// GatewayIndex.Gateways attaches policies, and policy IR resolves Secrets, so
// deriving refs from the index would make the Secret cache depend on a
// collection that depends on the Secret cache.
func GatewayResourceRefs(
	gateways krt.Collection[*gwv1.Gateway],
	listenerSets krt.Collection[*gwv1.ListenerSet],
	opts krtutil.KrtOptions,
) krt.Collection[ondemand.ResourceRef] {
	fromGateways := krt.NewManyCollection(gateways, func(kctx krt.HandlerContext, gw *gwv1.Gateway) []ondemand.ResourceRef {
		src := "Gateway/" + gw.Namespace + "/" + gw.Name
		refs := listenerCertRefs(src, gw.Namespace, gw.Spec.Listeners)

		if gw.Spec.TLS == nil {
			return ondemand.Dedupe(refs)
		}

		// spec.tls.backend.clientCertificateRef: the cert the Gateway presents to
		// backends.
		if b := gw.Spec.TLS.Backend; b != nil && b.ClientCertificateRef != nil {
			refs = append(refs, secretRefFrom(src, gw.Namespace, *b.ClientCertificateRef)...)
		}

		// spec.tls.frontend validation CA bundles, which may be ConfigMaps or
		// Secrets, both default and per-port.
		if f := gw.Spec.TLS.Frontend; f != nil {
			if v := f.Default.Validation; v != nil {
				refs = append(refs, caCertRefs(src, gw.Namespace, v.CACertificateRefs)...)
			}
			for _, pp := range f.PerPort {
				if v := pp.TLS.Validation; v != nil {
					refs = append(refs, caCertRefs(src, gw.Namespace, v.CACertificateRefs)...)
				}
			}
		}
		return ondemand.Dedupe(refs)
	}, opts.ToOptions("GatewayResourceRefs")...)

	fromListenerSets := krt.NewManyCollection(listenerSets, func(kctx krt.HandlerContext, ls *gwv1.ListenerSet) []ondemand.ResourceRef {
		src := "ListenerSet/" + ls.Namespace + "/" + ls.Name
		// ListenerSet listeners are a distinct type but carry the same TLS config.
		listeners := make([]gwv1.Listener, 0, len(ls.Spec.Listeners))
		for _, l := range ls.Spec.Listeners {
			listeners = append(listeners, gwv1.Listener{
				Name:     l.Name,
				Hostname: l.Hostname,
				Protocol: l.Protocol,
				TLS:      l.TLS,
			})
		}
		return ondemand.Dedupe(listenerCertRefs(src, ls.Namespace, listeners))
	}, opts.ToOptions("ListenerSetResourceRefs")...)

	return krt.JoinCollection(
		[]krt.Collection[ondemand.ResourceRef]{fromGateways, fromListenerSets},
		opts.ToOptions("CoreResourceRefs")...,
	)
}

// listenerCertRefs collects every certificateRef across a set of listeners.
func listenerCertRefs(source, defaultNamespace string, listeners []gwv1.Listener) []ondemand.ResourceRef {
	var refs []ondemand.ResourceRef
	for _, l := range listeners {
		if l.TLS == nil {
			continue
		}
		for _, certRef := range l.TLS.CertificateRefs {
			refs = append(refs, secretRefFrom(source, defaultNamespace, certRef)...)
		}
	}
	return refs
}

// caCertRefs converts CA bundle references, which may name either a ConfigMap
// or a Secret, into resource refs. This mirrors validateCAReferenceType: kinds
// other than those two are rejected during translation and need no fetch.
func caCertRefs(source, defaultNamespace string, refs []gwv1.ObjectReference) []ondemand.ResourceRef {
	out := make([]ondemand.ResourceRef, 0, len(refs))
	for _, ref := range refs {
		if string(ref.Group) != "" {
			continue
		}
		kind := string(ref.Kind)
		if kind != wellknown.ConfigMapKind && kind != wellknown.SecretKind {
			continue
		}
		ns := defaultNamespace
		if ref.Namespace != nil {
			ns = string(*ref.Namespace)
		}
		out = append(out, ondemand.NewRef(source, kind, ns, string(ref.Name)))
	}
	return out
}

// secretRefFrom converts a Gateway API SecretObjectReference into a ref, if it
// points at a core Secret. Other kinds are resolved by their own plugins.
func secretRefFrom(source, defaultNamespace string, ref gwv1.SecretObjectReference) []ondemand.ResourceRef {
	group := ""
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := wellknown.SecretKind
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	if group != "" || kind != wellknown.SecretKind {
		return nil
	}

	ns := defaultNamespace
	if ref.Namespace != nil {
		ns = string(*ref.Namespace)
	}
	// Refs are declared before ReferenceGrant is evaluated: a cross-namespace ref
	// that a grant later rejects costs one cached Secret, whereas fetching only
	// after the grant check would couple the cache to the grant collection.
	return []ondemand.ResourceRef{ondemand.NewRef(source, wellknown.SecretKind, ns, string(ref.Name))}
}
