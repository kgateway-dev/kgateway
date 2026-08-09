package collections

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// tcpRouteGVRs lists the TCPRoute API versions kgateway understands, most preferred first.
// TCPRoute is standard as of Gateway API v1.6; v1alpha2 is pre-promotion and only a
// candidate when experimental Gateway API features are enabled. See selectRouteGVRs.
var tcpRouteGVRs = []schema.GroupVersionResource{
	wellknown.TCPRouteV1GVR,
	wellknown.TCPRouteGVR,
}

func convertTCPRouteV1ToV1Alpha2(in *gwv1.TCPRoute) *gwv1a2.TCPRoute {
	if in == nil {
		return nil
	}

	return &gwv1a2.TCPRoute{
		// The Go type is normalized but the API version is not: TypeMeta records the version
		// this object was actually served as, which is the version its status must be written
		// back through. Erasing it here is what forced the write path to probe informers to
		// guess the version back.
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1.GroupVersion.String(),
			Kind:       wellknown.TCPRouteKind,
		},
		ObjectMeta: *in.ObjectMeta.DeepCopy(),
		Spec: gwv1a2.TCPRouteSpec{
			CommonRouteSpec: in.Spec.CommonRouteSpec,
			Rules:           convertTCPRouteRulesV1ToV1Alpha2(in.Spec.Rules),
		},
		// Status must be carried over: the declarative status writer compares the live
		// status on this converted object against the desired status to decide whether
		// a write is needed.
		Status: gwv1a2.TCPRouteStatus{
			RouteStatus: in.Status.RouteStatus,
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
