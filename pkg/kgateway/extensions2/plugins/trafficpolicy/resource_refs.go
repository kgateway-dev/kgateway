package trafficpolicy

import (
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/ondemand"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// resourceRefs declares the Secrets TrafficPolicy reads: API-key credentials
// (by name or by label selector), basic-auth htpasswd files, and secret-backed
// header values.
//
// It derives from the raw TrafficPolicy collection. Deriving from policyCol
// would deadlock: that collection resolves Secrets, and the Secret cache waits
// on these refs before it will serve any.
func resourceRefs(
	policies krt.Collection[*kgateway.TrafficPolicy],
	opts krtutil.KrtOptions,
) krt.Collection[ondemand.ResourceRef] {
	return krt.NewManyCollection(policies, func(kctx krt.HandlerContext, p *kgateway.TrafficPolicy) []ondemand.ResourceRef {
		src := "TrafficPolicy/" + p.Namespace + "/" + p.Name
		var refs []ondemand.ResourceRef

		// spec.apiKeyAuth: either one named Secret or a label selector. Selector
		// refs are expanded against the metadata watch, which carries labels.
		if ak := p.Spec.APIKeyAuth; ak != nil && ak.Disable == nil {
			switch {
			case ak.SecretRef != nil:
				ns := p.Namespace
				if ak.SecretRef.Namespace != nil {
					ns = string(*ak.SecretRef.Namespace)
				}
				refs = append(refs, ondemand.NewRef(src, wellknown.SecretKind, ns, string(ak.SecretRef.Name)))
			case ak.SecretSelector != nil:
				// GetSecretsBySelector searches every namespace and filters by
				// ReferenceGrant afterwards, so the ref must be cluster-wide too.
				refs = append(refs, ondemand.NewSelectorRef(src, wellknown.SecretKind, "", ak.SecretSelector.MatchLabels))
			}
		}

		// spec.basicAuth.secretRef
		if ba := p.Spec.BasicAuth; ba != nil && ba.SecretRef != nil {
			ns := p.Namespace
			if ba.SecretRef.Namespace != nil {
				ns = string(*ba.SecretRef.Namespace)
			}
			refs = append(refs, ondemand.NewRef(src, wellknown.SecretKind, ns, string(ba.SecretRef.Name)))
		}

		// spec.headerModifiers.{request,response}
		if hm := p.Spec.HeaderModifiers; hm != nil {
			refs = append(refs, pluginutils.HeaderFilterResourceRefs(src, p.Namespace, hm.Request)...)
			refs = append(refs, pluginutils.HeaderFilterResourceRefs(src, p.Namespace, hm.Response)...)
		}

		return ondemand.Dedupe(refs)
	}, opts.ToOptions("TrafficPolicyResourceRefs")...)
}
