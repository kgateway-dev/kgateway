package backend

import (
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/ondemand"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// resourceRefs declares the AWS credential Secrets a Backend reads. Derived
// from the raw Backend collection; see the ondemand package for why it must not
// come from the translated one.
func resourceRefs(
	backends krt.Collection[*kgateway.Backend],
	opts krtutil.KrtOptions,
) krt.Collection[ondemand.ResourceRef] {
	return krt.NewManyCollection(backends, func(kctx krt.HandlerContext, b *kgateway.Backend) []ondemand.ResourceRef {
		aws := b.Spec.Aws
		if aws == nil || aws.Auth == nil ||
			aws.Auth.Type != kgateway.AwsAuthTypeSecret || aws.Auth.SecretRef == nil {
			return nil
		}
		// loadAWSSecret resolves in the Backend's own namespace.
		return []ondemand.ResourceRef{
			ondemand.NewRef("Backend/"+b.Namespace+"/"+b.Name, wellknown.SecretKind, b.Namespace, aws.Auth.SecretRef.Name),
		}
	}, opts.ToOptions("BackendResourceRefs")...)
}
