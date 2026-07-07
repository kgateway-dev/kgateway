package collections

import (
	"context"

	"istio.io/istio/pkg/config/schema/gvr"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

var promotedTCPRouteGVR = wellknown.TCPRouteV1GVR

type servedTCPRouteVersions struct {
	Promoted      bool
	PreV1         bool
	Authoritative bool
}

func fallbackTCPRouteVersions() servedTCPRouteVersions {
	return servedTCPRouteVersions{
		Promoted: true,
		PreV1:    true,
	}
}

// preV1TCPRouteWatchGVRs returns the pre-v1 TCPRoute API versions that should
// be watched for the current discovery result. When discovery is authoritative
// and the promoted v1 version is served, skip the pre-v1 watch to avoid
// duplicate logical TCPRoutes.
func preV1TCPRouteWatchGVRs(versions servedTCPRouteVersions) []schema.GroupVersionResource {
	if !versions.PreV1 || (versions.Authoritative && versions.Promoted) {
		return nil
	}
	return []schema.GroupVersionResource{gvr.TCPRoute}
}

// getServedTCPRouteVersions resolves which TCPRoute API versions are currently
// served by the cluster. When discovery is unavailable, or the CRD is not yet
// installed, we conservatively allow both promoted and pre-v1 watches so
// startup does not incorrectly disable TCPRoute support before delayed
// informers can recover.
func getServedTCPRouteVersions(extClient apiextensionsclient.Interface) servedTCPRouteVersions {
	if extClient == nil {
		// If discovery is unavailable, keep both paths enabled and let the delayed
		// informer logic determine what is actually readable at runtime.
		return fallbackTCPRouteVersions()
	}

	ctx, cancel := context.WithTimeout(context.Background(), crdLookupTimeout)
	defer cancel()

	crd, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, wellknown.TCPRouteCRDName, metav1.GetOptions{})
	if err != nil {
		return fallbackTCPRouteVersions()
	}

	versions := servedTCPRouteVersions{Authoritative: true}
	for _, version := range crd.Spec.Versions {
		if !version.Served {
			continue
		}

		switch version.Name {
		case gwv1.GroupVersion.Version:
			versions.Promoted = true
		case gwv1a2.GroupVersion.Version:
			versions.PreV1 = true
		}
	}

	return versions
}

func convertTCPRouteV1ToV1Alpha2(in *gwv1.TCPRoute) *gwv1a2.TCPRoute {
	if in == nil {
		return nil
	}

	return &gwv1a2.TCPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1a2.GroupVersion.String(),
			Kind:       wellknown.TCPRouteKind,
		},
		ObjectMeta: *in.ObjectMeta.DeepCopy(),
		Spec: gwv1a2.TCPRouteSpec{
			CommonRouteSpec: in.Spec.CommonRouteSpec,
			Rules:           convertTCPRouteRulesV1ToV1Alpha2(in.Spec.Rules),
		},
	}
}

func convertTCPRouteRulesV1ToV1Alpha2(in []gwv1.TCPRouteRule) []gwv1a2.TCPRouteRule {
	if len(in) == 0 {
		return nil
	}

	out := make([]gwv1a2.TCPRouteRule, 0, len(in))
	for _, rule := range in {
		out = append(out, gwv1a2.TCPRouteRule(rule))
	}
	return out
}
