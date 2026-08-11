package listenerpolicy

import (
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	kgwwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/ondemand"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// resourceRefs declares what ListenerPolicy reads: secret-backed local-reply
// headers, and the CA bundles used for client certificate validation.
//
// The CA bundles are resolved during Gateway listener translation rather than
// in this plugin's IR, but they originate here, so this is where the reference
// is declared.
//
// Derived from the raw policy collection to keep the Secret and ConfigMap
// caches free of any dependency on collections that read them.
func resourceRefs(
	policies krt.Collection[*kgateway.ListenerPolicy],
	opts krtutil.KrtOptions,
) krt.Collection[ondemand.ResourceRef] {
	return krt.NewManyCollection(policies, func(kctx krt.HandlerContext, p *kgateway.ListenerPolicy) []ondemand.ResourceRef {
		src := "ListenerPolicy/" + p.Namespace + "/" + p.Name
		var refs []ondemand.ResourceRef

		// Listener config appears once as the default and again per port; both
		// carry local-reply headers, so collect from every occurrence.
		if d := p.Spec.Default; d != nil {
			refs = append(refs, localReplyRefs(src, p.Namespace, &d.ListenerConfig)...)
			// Client certificate validation is default-only today.
			refs = append(refs, caCertRefs(src, p.Namespace, d.ClientCertificateValidation)...)
		}
		for i := range p.Spec.PerPort {
			refs = append(refs, localReplyRefs(src, p.Namespace, &p.Spec.PerPort[i].Listener)...)
		}

		return ondemand.Dedupe(refs)
	}, opts.ToOptions("ListenerPolicyResourceRefs")...)
}

// localReplyRefs collects the Secrets backing local-reply header values.
func localReplyRefs(source, namespace string, cfg *kgateway.ListenerConfig) []ondemand.ResourceRef {
	if cfg == nil || cfg.HTTPSettings == nil || cfg.HTTPSettings.LocalReplies == nil {
		return nil
	}
	var refs []ondemand.ResourceRef
	for _, m := range cfg.HTTPSettings.LocalReplies.Mappers {
		refs = append(refs, pluginutils.HeaderFilterResourceRefs(source, namespace, m.Headers)...)
	}
	return refs
}

// caCertRefs collects the CA bundles used for client certificate validation.
// They may be ConfigMaps or Secrets; other kinds are rejected in translation.
func caCertRefs(source, namespace string, cv *kgateway.ClientCertificateValidationConfig) []ondemand.ResourceRef {
	if cv == nil {
		return nil
	}
	refs := make([]ondemand.ResourceRef, 0, len(cv.CACertificateRefs))
	for _, ref := range cv.CACertificateRefs {
		if string(ref.Group) != "" {
			continue
		}
		kind := string(ref.Kind)
		if kind != kgwwellknown.ConfigMapKind && kind != kgwwellknown.SecretKind {
			continue
		}
		ns := namespace
		if ref.Namespace != nil {
			ns = string(*ref.Namespace)
		}
		refs = append(refs, ondemand.NewRef(source, kind, ns, string(ref.Name)))
	}
	return refs
}
