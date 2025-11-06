package query

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func ProcessBackendError(err error, reporter reports.ParentRefReporter) {
	switch {
	case errors.Is(err, krtcollections.ErrUnknownBackendKind):
		reporter.SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonInvalidKind,
			Message: err.Error(),
		})
	case errors.Is(err, &krtcollections.BackendPortNotAllowedError{}):
		reporter.SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonBackendNotFound,
			Message: err.Error(),
		})
	case errors.Is(err, krtcollections.ErrMissingReferenceGrant):
		reporter.SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonRefNotPermitted,
			Message: err.Error(),
		})
	case errors.Is(err, ErrCyclicReference):
		reporter.SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonRefNotPermitted,
			Message: err.Error(),
		})
	case errors.Is(err, ErrUnresolvedReference):
		reporter.SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonBackendNotFound,
			Message: err.Error(),
		})
	case apierrors.IsNotFound(err):
		reporter.SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonBackendNotFound,
			Message: err.Error(),
		})
	default:
		// setting other errors to not found. not sure if there's a better option.
		reporter.SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonBackendNotFound,
			Message: err.Error(),
		})
	}
}
