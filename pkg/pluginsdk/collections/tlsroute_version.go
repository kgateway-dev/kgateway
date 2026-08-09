package collections

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		// The Go type is normalized but the API version is not: TypeMeta records the version
		// this object was actually served as, which is the version its status must be written
		// back through. Erasing it here is what forced the write path to probe informers to
		// guess the version back.
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1.GroupVersion.String(),
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
		// See convertTLSRouteV1ToV1Alpha2: the served API version is preserved.
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1a3.GroupVersion.String(),
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
