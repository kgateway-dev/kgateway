package collections

import (
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

var logger = logging.New("pluginsdk/collections")

// tlsRouteGVRs lists the TLSRoute API versions kgateway understands, most preferred first.
// TLSRoute is standard as of Gateway API v1.5; v1alpha3 and v1alpha2 are pre-promotion and
// only candidates when experimental Gateway API features are enabled. v1alpha3 outranks
// v1alpha2 so a cluster serving both (Gateway API v1.4.1) settles on the newer of the two.
// See selectRouteGVRs.
var tlsRouteGVRs = []schema.GroupVersionResource{
	wellknown.TLSRouteV1GVR,
	wellknown.TLSRouteV1Alpha3GVR,
	wellknown.TLSRouteGVR,
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
		Status: gwv1a2.TLSRouteStatus{
			RouteStatus: in.Status.RouteStatus,
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
		logger.Warn("ignoring unstructured TLSRoute with invalid payload",
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
