package reports

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

var acceptedGWNN = types.NamespacedName{Namespace: "default", Name: "gw"}

// deployerVerdict is what the Gateway deployer published for the Gateway, if anything.
type deployerVerdict struct {
	generation int64
	err        error
}

// reduceGateway reduces the translation report and the deployer's verdict for the Gateway the
// way the status syncer does: as two contributions to the same target.
func reduceGateway(t *testing.T, gw *gwv1.Gateway, noValidListeners bool, verdict *deployerVerdict) *GatewayReport {
	t.Helper()
	rm := NewReportMap()
	gwReporter := NewReporter(&rm).Gateway(gw)
	gwReporter.Listener(&gw.Spec.Listeners[0]).SetCondition(reporter.ListenerCondition{
		Type:    gwv1.ListenerConditionProgrammed,
		Status:  metav1.ConditionFalse,
		Reason:  gwv1.ListenerReasonInvalid,
		Message: "reserved port",
	})
	if noValidListeners {
		gwReporter.SetCondition(reporter.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: "No valid listeners",
		})
	}
	contributions := StatusContributionsFromReportMap(StatusSource{Kind: GatewayStatusSource, Name: "default/gw"}, rm)
	if verdict != nil {
		params := NewGatewayParametersStatus()
		params.Set(acceptedGWNN, verdict.generation, verdict.err)
		contributions = append(contributions, params.Contributions().List()...)
	}
	return ReduceStatusContributions(contributions).Gateway
}

func acceptedCondition(generation int64, status metav1.ConditionStatus, reason gwv1.GatewayConditionReason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               string(gwv1.GatewayConditionAccepted),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: generation,
	}
}

// The status writer is the only writer of the Gateway Accepted condition; the deployer's
// verdict on the parameters is one of its inputs.
func TestBuildGWStatusAcceptedWithDeployerVerdict(t *testing.T) {
	invalidParams := errors.New("invalid parametersRef")
	listenersNotValid := "Some listeners are not programmed: http: reserved port"
	tests := []struct {
		name             string
		generation       int64
		live             *metav1.Condition
		noValidListeners bool
		verdict          *deployerVerdict
		want             metav1.Condition
	}{
		{
			name:       "a failed verdict wins over an unprogrammed listener",
			generation: 1,
			verdict:    &deployerVerdict{generation: 1, err: invalidParams},
			want:       acceptedCondition(1, metav1.ConditionFalse, gwv1.GatewayReasonInvalidParameters, invalidParams.Error()),
		},
		{
			name:             "a failed verdict wins over the report's own Accepted",
			generation:       1,
			noValidListeners: true,
			verdict:          &deployerVerdict{generation: 1, err: invalidParams},
			want:             acceptedCondition(1, metav1.ConditionFalse, gwv1.GatewayReasonInvalidParameters, invalidParams.Error()),
		},
		{
			name:       "a successful verdict clears InvalidParameters for ListenersNotValid",
			generation: 1,
			live:       new(acceptedCondition(1, metav1.ConditionFalse, gwv1.GatewayReasonInvalidParameters, invalidParams.Error())),
			verdict:    &deployerVerdict{generation: 1},
			want:       acceptedCondition(1, metav1.ConditionTrue, gwv1.GatewayReasonListenersNotValid, listenersNotValid),
		},
		{
			name:       "before the deployer's first pass, InvalidParameters is kept",
			generation: 1,
			live:       new(acceptedCondition(1, metav1.ConditionFalse, gwv1.GatewayReasonInvalidParameters, invalidParams.Error())),
			want:       acceptedCondition(1, metav1.ConditionFalse, gwv1.GatewayReasonInvalidParameters, invalidParams.Error()),
		},
		{
			name:       "a verdict on an older generation does not clear InvalidParameters",
			generation: 2,
			live:       new(acceptedCondition(1, metav1.ConditionFalse, gwv1.GatewayReasonInvalidParameters, invalidParams.Error())),
			verdict:    &deployerVerdict{generation: 1},
			want:       acceptedCondition(1, metav1.ConditionFalse, gwv1.GatewayReasonInvalidParameters, invalidParams.Error()),
		},
		{
			name:       "a failed verdict on an older generation is not reported against a newer one",
			generation: 2,
			live:       new(acceptedCondition(1, metav1.ConditionTrue, gwv1.GatewayReasonListenersNotValid, listenersNotValid)),
			verdict:    &deployerVerdict{generation: 1, err: invalidParams},
			want:       acceptedCondition(2, metav1.ConditionTrue, gwv1.GatewayReasonListenersNotValid, listenersNotValid),
		},
		{
			name:       "a failed verdict is stamped with the generation the deployer reconciled",
			generation: 2,
			verdict:    &deployerVerdict{generation: 3, err: invalidParams},
			want:       acceptedCondition(3, metav1.ConditionFalse, gwv1.GatewayReasonInvalidParameters, invalidParams.Error()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Namespace: acceptedGWNN.Namespace, Name: acceptedGWNN.Name, Generation: tt.generation},
				Spec:       gwv1.GatewaySpec{Listeners: []gwv1.Listener{{Name: "http", Port: 9091}}},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{{Type: "example.com/Foreign", Status: metav1.ConditionTrue, Reason: "Foreign"}},
					Listeners:  []gwv1.ListenerStatus{{Name: "http", SupportedKinds: []gwv1.RouteGroupKind{}}},
				},
			}
			if tt.live != nil {
				gw.Status.Conditions = append(gw.Status.Conditions, *tt.live)
			}
			report := reduceGateway(t, gw, tt.noValidListeners, tt.verdict)

			status := BuildGWStatus(report, *gw, nil)
			require.NotNil(t, status)
			got := meta.FindStatusCondition(status.Conditions, string(gwv1.GatewayConditionAccepted))
			require.NotNil(t, got, "Accepted condition should be set")
			require.Equal(t, tt.want.Status, got.Status)
			require.Equal(t, tt.want.Reason, got.Reason)
			require.Equal(t, tt.want.Message, got.Message)
			require.Equal(t, tt.want.ObservedGeneration, got.ObservedGeneration)
			require.NotNil(t, meta.FindStatusCondition(status.Conditions, "example.com/Foreign"),
				"conditions we do not own must survive")

			// Writing the result back and rebuilding must not change it again; otherwise every
			// write would trigger another.
			gw.Status = *status
			require.Equal(t, status, BuildGWStatus(report, *gw, nil), "status should be a fixed point")
		})
	}
}

// verdictFor reads the deployer's verdict for gw from the contributions it publishes.
func verdictFor(params *GatewayParametersStatus, gw types.NamespacedName) *GatewayParametersReport {
	for _, contribution := range params.Contributions().List() {
		if contribution.Target.NamespacedName == gw && contribution.GatewayParameters != nil {
			return contribution.GatewayParameters
		}
	}
	return nil
}

func TestGatewayParametersStatus(t *testing.T) {
	params := NewGatewayParametersStatus()
	require.Nil(t, verdictFor(params, acceptedGWNN), "nothing is recorded before the deployer reconciles the Gateway")

	params.Set(acceptedGWNN, 1, errors.New("invalid parametersRef"))
	verdict := verdictFor(params, acceptedGWNN)
	require.NotNil(t, verdict)
	require.Equal(t, int64(1), verdict.ObservedGeneration())
	require.Equal(t, "invalid parametersRef", verdict.Err())

	params.Set(acceptedGWNN, 2, nil)
	verdict = verdictFor(params, acceptedGWNN)
	require.NotNil(t, verdict)
	require.Equal(t, int64(2), verdict.ObservedGeneration())
	require.Empty(t, verdict.Err())
	require.Len(t, params.Contributions().List(), 1, "a Gateway has one verdict")

	require.Nil(t, ReduceStatusContributions(params.Contributions().List()).Gateway,
		"a verdict alone, before translation reports on the Gateway, writes nothing")

	params.Remove(acceptedGWNN)
	require.Nil(t, verdictFor(params, acceptedGWNN))

	var unset *GatewayParametersStatus
	unset.Set(acceptedGWNN, 1, errors.New("ignored"))
	unset.Remove(acceptedGWNN)
}

func TestGatewayParametersReportEqual(t *testing.T) {
	base := &GatewayParametersReport{observedGeneration: 1, err: "invalid"}
	require.True(t, gatewayParametersReportEqual(base, &GatewayParametersReport{observedGeneration: 1, err: "invalid"}))
	require.False(t, gatewayParametersReportEqual(base, &GatewayParametersReport{observedGeneration: 2, err: "invalid"}),
		"Equals should compare observedGeneration")
	require.False(t, gatewayParametersReportEqual(base, &GatewayParametersReport{observedGeneration: 1}),
		"Equals should compare err")
	require.False(t, gatewayParametersReportEqual(base, nil))

	report := &GatewayReport{parameters: base}
	require.False(t, gatewayReportEqual(report, &GatewayReport{}), "GatewayReport equality should compare parameters")
}
