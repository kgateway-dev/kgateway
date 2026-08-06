package query

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

type conditionRecorder struct {
	condition reporter.RouteCondition
}

func (r *conditionRecorder) SetCondition(condition reporter.RouteCondition) {
	r.condition = condition
}

func TestProcessBackendErrorPreservesServiceSelectorDiagnostic(t *testing.T) {
	recorder := &conditionRecorder{}
	err := &krtcollections.ServiceBackendNotFoundError{
		NotFound: &krtcollections.NotFoundError{NotFoundObj: ir.ObjectSource{
			Kind:      "Service",
			Namespace: "default",
			Name:      "api",
		}},
		ServiceLabelSelector: "exposure=public",
	}

	ProcessBackendError(err, recorder)

	require.Equal(t, gwv1.RouteConditionResolvedRefs, recorder.condition.Type)
	require.Equal(t, metav1.ConditionFalse, recorder.condition.Status)
	require.Equal(t, gwv1.RouteReasonBackendNotFound, recorder.condition.Reason)
	require.Equal(t, err.Error(), recorder.condition.Message)
	require.Contains(t, recorder.condition.Message, "controller.discovery.serviceLabelSelector")
	require.Contains(t, recorder.condition.Message, `"exposure=public"`)
}
