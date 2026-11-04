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
		// Every served TCPRoute version is normalized to this one Go type, so TypeMeta is the
		// only record of which version an object came from — and that is the version its
		// status must be written back through. statussync.RegisterKindByObjectGVK keys the
		// status reductions and the write queue off this GVK, so it has to be set here.
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
	return convertRouteSliceElems(in, func(r gwv1.TCPRouteRule) gwv1a2.TCPRouteRule {
		return gwv1a2.TCPRouteRule(r)
	})
}

// convertRouteSliceElems converts a slice of route spec elements to the identically-shaped
// alpha type. Go has no conversion between slices of distinct element types, so every one of
// these conversions is element-wise; this holds the part of that which is easy to get wrong.
//
// Empty input converts to nil, whether it arrived as nil or as a non-nil empty slice: the
// two are canonicalized to one. That is what each of the per-kind helpers folded in here
// already did, so it is preserved rather than chosen. It is also why this does not use
// slices.Map, which returns a non-nil empty slice for empty input — switching would flip the
// normalized Spec of every route that carries no rules or hostnames, which is a behavior
// change unrelated to deduplicating these conversions.
func convertRouteSliceElems[In, Out any](in []In, convert func(In) Out) []Out {
	if len(in) == 0 {
		return nil
	}
	out := make([]Out, 0, len(in))
	for _, e := range in {
		out = append(out, convert(e))
	}
	return out
}
