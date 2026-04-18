package collections

import (
	"context"
	"log/slog"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

var promotedTLSRouteGVR = schema.GroupVersionResource{
	Group:    wellknown.GatewayGroup,
	Version:  gwv1.GroupVersion.Version,
	Resource: "tlsroutes",
}

var tlsRouteV1Alpha3GVR = schema.GroupVersionResource{
	Group:    wellknown.GatewayGroup,
	Version:  wellknown.TLSRouteV1Alpha3Version,
	Resource: "tlsroutes",
}

var tlsRouteV1Alpha2GVR = schema.GroupVersionResource{
	Group:    wellknown.GatewayGroup,
	Version:  gwv1a2.GroupVersion.Version,
	Resource: "tlsroutes",
}

type servedTLSRouteVersions struct {
	Promoted          bool
	PreV1             bool
	PreferredPreV1GVR schema.GroupVersionResource
	Authoritative     bool
}

func fallbackTLSRouteVersions() servedTLSRouteVersions {
	return servedTLSRouteVersions{
		Promoted:          true,
		PreV1:             true,
		PreferredPreV1GVR: tlsRouteV1Alpha3GVR,
	}
}

// preV1TLSRouteWatchGVRs returns the pre-v1 TLSRoute API versions that should
// be watched for the current discovery result. When discovery is authoritative,
// prefer a single served version to avoid duplicate logical TLSRoutes. When
// discovery is non-authoritative, keep both pre-v1 versions active so clusters
// that only serve v1alpha2 remain discoverable.
func preV1TLSRouteWatchGVRs(versions servedTLSRouteVersions) []schema.GroupVersionResource {
	if !versions.PreV1 || (versions.Authoritative && versions.Promoted) {
		return nil
	}
	if versions.Authoritative {
		return []schema.GroupVersionResource{versions.PreferredPreV1GVR}
	}
	return []schema.GroupVersionResource{tlsRouteV1Alpha3GVR, tlsRouteV1Alpha2GVR}
}

// getServedTLSRouteVersions resolves which TLSRoute API versions are currently
// served by the cluster. When discovery is unavailable, or the CRD is not yet
// installed, we conservatively allow both promoted and pre-v1 watches so
// startup does not incorrectly disable TLSRoute support before delayed
// informers can recover.
func getServedTLSRouteVersions(extClient apiextensionsclient.Interface) servedTLSRouteVersions {
	if extClient == nil {
		// If discovery is unavailable, keep both paths enabled and let the delayed
		// informer logic determine what is actually readable at runtime.
		return fallbackTLSRouteVersions()
	}

	ctx, cancel := context.WithTimeout(context.Background(), crdLookupTimeout)
	defer cancel()

	crd, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, "tlsroutes.gateway.networking.k8s.io", metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fallbackTLSRouteVersions()
		}
		return fallbackTLSRouteVersions()
	}

	versions := servedTLSRouteVersions{Authoritative: true}
	servedPreV1Versions := map[string]bool{}
	for _, version := range crd.Spec.Versions {
		if !version.Served {
			continue
		}

		switch version.Name {
		case gwv1.GroupVersion.Version:
			versions.Promoted = true
		case wellknown.TLSRouteV1Alpha3Version, gwv1a2.GroupVersion.Version:
			servedPreV1Versions[version.Name] = true
		}
	}

	// Prefer v1alpha3 over v1alpha2 when both pre-v1 versions are served so we
	// consistently watch the most recent pre-promotion API and avoid duplicate
	// logical TLSRoutes from multiple pre-v1 watches.
	for _, preV1Version := range []string{wellknown.TLSRouteV1Alpha3Version, gwv1a2.GroupVersion.Version} {
		if servedPreV1Versions[preV1Version] {
			versions.PreV1 = true
			versions.PreferredPreV1GVR = schema.GroupVersionResource{
				Group:    wellknown.GatewayGroup,
				Version:  preV1Version,
				Resource: "tlsroutes",
			}
			break
		}
	}

	return versions
}

func convertTLSRouteV1ToV1Alpha2(in *gwv1.TLSRoute) *gwv1a2.TLSRoute {
	if in == nil {
		return nil
	}

	return &gwv1a2.TLSRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1a2.GroupVersion.String(),
			Kind:       wellknown.TLSRouteKind,
		},
		ObjectMeta: *in.ObjectMeta.DeepCopy(),
		Spec: gwv1a2.TLSRouteSpec{
			CommonRouteSpec: gwv1a2.CommonRouteSpec{
				ParentRefs:         in.Spec.ParentRefs,
				UseDefaultGateways: in.Spec.UseDefaultGateways,
			},
			Hostnames: convertTLSRouteHostnamesV1ToV1Alpha2(in.Spec.Hostnames),
			Rules:     convertTLSRouteRulesV1ToV1Alpha2(in.Spec.Rules),
		},
	}
}

func convertTLSRouteV1Alpha3ToV1Alpha2(in *gwv1a3.TLSRoute) *gwv1a2.TLSRoute {
	if in == nil {
		return nil
	}

	return &gwv1a2.TLSRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1a2.GroupVersion.String(),
			Kind:       wellknown.TLSRouteKind,
		},
		ObjectMeta: *in.ObjectMeta.DeepCopy(),
		Spec: gwv1a2.TLSRouteSpec{
			CommonRouteSpec: gwv1a2.CommonRouteSpec{
				ParentRefs:         in.Spec.ParentRefs,
				UseDefaultGateways: in.Spec.UseDefaultGateways,
			},
			Hostnames: convertTLSRouteHostnamesV1ToV1Alpha2(in.Spec.Hostnames),
			Rules:     convertTLSRouteRulesV1ToV1Alpha2(in.Spec.Rules),
		},
		Status: gwv1a2.TLSRouteStatus{
			RouteStatus: in.Status.RouteStatus,
		},
	}
}

func convertUnstructuredTLSRouteToV1Alpha2(in *unstructured.Unstructured) *gwv1a2.TLSRoute {
	if in == nil {
		return nil
	}

	out := &gwv1a2.TLSRoute{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(in.Object, out); err != nil {
		slog.Warn("ignoring unstructured TLSRoute with invalid payload",
			"name", in.GetName(),
			"namespace", in.GetNamespace(),
			"error", err,
		)
		return nil
	}
	out.SetGroupVersionKind(wellknown.TLSRouteGVK)
	return out
}

// ConvertUnstructuredTLSRouteToV1Alpha2ForStatus normalizes TLSRoute objects
// fetched as *unstructured.Unstructured by getTLSRouteForStatus. Status sync
// uses an unstructured Get against the controller-runtime manager client for
// TLSRoute versions that are not registered in the manager scheme (today:
// v1alpha3 — see pkg/schemes/scheme.go). This helper converts that
// unstructured object into *gwv1a2.TLSRoute so the existing gwv1a2-typed
// status report builder can process it.
func ConvertUnstructuredTLSRouteToV1Alpha2ForStatus(in *unstructured.Unstructured) *gwv1a2.TLSRoute {
	return convertUnstructuredTLSRouteToV1Alpha2(in)
}

func convertTLSRouteHostnamesV1ToV1Alpha2(in []gwv1.Hostname) []gwv1a2.Hostname {
	if len(in) == 0 {
		return nil
	}

	out := make([]gwv1a2.Hostname, 0, len(in))
	for _, hostname := range in {
		out = append(out, gwv1a2.Hostname(hostname))
	}
	return out
}

func convertTLSRouteRulesV1ToV1Alpha2(in []gwv1.TLSRouteRule) []gwv1a2.TLSRouteRule {
	if len(in) == 0 {
		return nil
	}

	out := make([]gwv1a2.TLSRouteRule, 0, len(in))
	for _, rule := range in {
		out = append(out, gwv1a2.TLSRouteRule(rule))
	}
	return out
}
