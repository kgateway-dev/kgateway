package query

import (
	"errors"

	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ResolveBackendRef(cli client.Object, err error, reporter reports.ParentRefReporter, backendRef gwv1.BackendObjectReference) *string {
	if err != nil {
		switch {
		case errors.Is(err, ErrUnknownKind):
			reporter.SetCondition(reports.HTTPRouteCondition{
				Type:   gwv1.RouteConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonInvalidKind,
			})
		case errors.Is(err, ErrMissingReferenceGrant):
			reporter.SetCondition(reports.HTTPRouteCondition{
				Type:   gwv1.RouteConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonRefNotPermitted,
			})
		case apierrors.IsNotFound(err):
			reporter.SetCondition(reports.HTTPRouteCondition{
				Type:   gwv1.RouteConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonBackendNotFound,
			})
		default:
			// setting other errors to not found. not sure if there's a better option.
			reporter.SetCondition(reports.HTTPRouteCondition{
				Type:   gwv1.RouteConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonBackendNotFound,
			})
		}
	} else {
		var port uint32
		if backendRef.Port != nil {
			port = uint32(*backendRef.Port)
		}
		switch cli := cli.(type) {
		case *corev1.Service:
			if port == 0 {
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonUnsupportedValue,
				})
			} else {
				name := kubernetes.UpstreamName(cli.Namespace, cli.Name, int32(port))
				return &name
			}
		default:
			reporter.SetCondition(reports.HTTPRouteCondition{
				Type:   gwv1.RouteConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonInvalidKind,
			})
		}
	}
	return nil
}
