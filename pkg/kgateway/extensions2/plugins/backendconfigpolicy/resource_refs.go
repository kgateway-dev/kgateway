package backendconfigpolicy

import (
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/ondemand"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// resourceRefs declares the client-certificate Secrets BackendConfigPolicy
// reads for upstream TLS. Derived from the raw policy collection; see the
// ondemand package for why it must not come from the translated one.
func resourceRefs(
	policies krt.Collection[*kgateway.BackendConfigPolicy],
	opts krtutil.KrtOptions,
) krt.Collection[ondemand.ResourceRef] {
	return krt.NewManyCollection(policies, func(kctx krt.HandlerContext, p *kgateway.BackendConfigPolicy) []ondemand.ResourceRef {
		tls := p.Spec.TLS
		if tls == nil || tls.SecretRef == nil {
			return nil
		}
		src := "BackendConfigPolicy/" + p.Namespace + "/" + p.Name
		// SecretRef is a LocalObjectReference, resolved in the policy's namespace.
		return []ondemand.ResourceRef{
			ondemand.NewRef(src, wellknown.SecretKind, p.Namespace, tls.SecretRef.Name),
		}
	}, opts.ToOptions("BackendConfigPolicyResourceRefs")...)
}
