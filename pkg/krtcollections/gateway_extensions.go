package krtcollections

import (
	"context"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/ondemand"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
)

func NewGatewayExtensionsCollection(
	ctx context.Context,
	client apiclient.Client,
	krtOpts krtutil.KrtOptions,
) (krt.Collection[ir.GatewayExtension], krt.Collection[*kgateway.GatewayExtension]) {
	rawGwExts := krt.WrapClient(kclient.NewFilteredDelayed[*kgateway.GatewayExtension](
		client,
		wellknown.GatewayExtensionGVR,
		kclient.Filter{ObjectFilter: client.ObjectFilter()},
	), krtOpts.ToOptions("GatewayExtension")...)
	gwExtCol := krt.NewCollection(rawGwExts, func(krtctx krt.HandlerContext, cr *kgateway.GatewayExtension) *ir.GatewayExtension {
		weight, err := pluginsdkutils.ParsePrecedenceWeightAnnotation(cr.Annotations, apiannotations.PolicyPrecedenceWeight)
		if err != nil {
			logger.Error("error parsing precedence weight annotation; will default to 0", "resource_ref", ctrlclient.ObjectKeyFromObject(cr), "error", err)
		}
		gwExt := &ir.GatewayExtension{
			ObjectSource: ir.ObjectSource{
				Group:     wellknown.GatewayExtensionGVK.GroupKind().Group,
				Kind:      wellknown.GatewayExtensionGVK.GroupKind().Kind,
				Namespace: cr.Namespace,
				Name:      cr.Name,
			},
			ExtAuth:          cr.Spec.ExtAuth,
			ExtProc:          cr.Spec.ExtProc,
			RateLimit:        cr.Spec.RateLimit,
			JWT:              cr.Spec.JWT,
			OAuth2:           cr.Spec.OAuth2,
			PrecedenceWeight: weight,
		}
		return gwExt
	})
	return gwExtCol, rawGwExts
}

// GatewayExtensionResourceRefs declares the Secrets and ConfigMaps that
// GatewayExtension-backed policies read: OAuth2 client credentials, the OAuth2
// HMAC signing key, and locally-stored JWKS.
//
// Derived from the raw GatewayExtension collection. The resolution itself lives
// in the trafficpolicy plugin, but the references are visible from the CRD
// alone, and deriving them here keeps the Secret cache independent of any
// collection that reads it.
func GatewayExtensionResourceRefs(
	rawGwExts krt.Collection[*kgateway.GatewayExtension],
	krtOpts krtutil.KrtOptions,
) krt.Collection[ondemand.ResourceRef] {
	return krt.NewManyCollection(rawGwExts, func(krtctx krt.HandlerContext, cr *kgateway.GatewayExtension) []ondemand.ResourceRef {
		src := "GatewayExtension/" + cr.Namespace + "/" + cr.Name
		var refs []ondemand.ResourceRef

		if o := cr.Spec.OAuth2; o != nil {
			// Client secret is a LocalObjectReference: same namespace as the
			// GatewayExtension.
			refs = append(refs, ondemand.NewRef(src, wellknown.SecretKind, cr.Namespace,
				o.Credentials.ClientSecretRef.Name))
			// Every OAuth2 provider also reads the shared HMAC key, which the
			// bootstrap controller creates at a fixed location.
			refs = append(refs, ondemand.NewRef(src, wellknown.SecretKind,
				wellknown.OAuth2HMACSecret.Namespace, wellknown.OAuth2HMACSecret.Name))
		}

		if j := cr.Spec.JWT; j != nil {
			for _, p := range j.Providers {
				if l := p.JWKS.LocalJWKS; l != nil && l.ConfigMapRef != nil {
					refs = append(refs, ondemand.NewRef(src, wellknown.ConfigMapKind, cr.Namespace, l.ConfigMapRef.Name))
				}
			}
		}

		return ondemand.Dedupe(refs)
	}, krtOpts.ToOptions("GatewayExtensionResourceRefs")...)
}
