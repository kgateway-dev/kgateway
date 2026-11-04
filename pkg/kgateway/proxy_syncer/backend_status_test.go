package proxy_syncer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func backendSource(name string) ir.ObjectSource {
	return ir.ObjectSource{
		Group:     wellknown.BackendGVK.Group,
		Kind:      wellknown.BackendGVK.Kind,
		Namespace: "default",
		Name:      name,
	}
}

func kgwBackend(name string, generation int64, errs ...error) ir.BackendObjectIR {
	src := backendSource(name)
	b := ir.NewBackendObjectIR(src, 0, "", "")
	b.Obj = &metav1.ObjectMeta{Namespace: src.Namespace, Name: src.Name, Generation: generation}
	b.Errors = errs
	return b
}

func TestGenerateBackendStatusReport(t *testing.T) {
	a := assert.New(t)

	// accepted: no IR errors, translated successfully for two clients
	accepted := kgwBackend("accepted", 3)
	// irError: has an IR-construction error (reported even without a client)
	irError := kgwBackend("ir-error", 4, errors.New("Secret default/foo not found"))
	// translationError: no IR errors, but per-client translation failed
	translationError := kgwBackend("translation-error", 5)

	backends := []ir.BackendObjectIR{accepted, irError, translationError}

	clusters := []uccWithCluster{
		{BackendSource: backendSource("accepted"), BackendGeneration: 3},
		{BackendSource: backendSource("accepted"), BackendGeneration: 3},
		// per-client translation error for translation-error backend, deduped across clients
		{BackendSource: backendSource("translation-error"), BackendGeneration: 5, Error: errors.New("policy is invalid")},
		{BackendSource: backendSource("translation-error"), BackendGeneration: 5, Error: errors.New("policy is invalid")},
	}

	rm := GenerateBackendStatusReport(backends, clusters, nil)
	a.Len(rm.Backends, 3)

	acc := rm.Backends[types.NamespacedName{Namespace: "default", Name: "accepted"}]
	a.NotNil(acc)
	cond := meta.FindStatusCondition(acc.GetConditions(), "Accepted")
	a.NotNil(cond)
	a.Equal(metav1.ConditionTrue, cond.Status)
	a.Equal("Accepted", cond.Reason)
	a.Equal("Backend accepted", cond.Message)
	a.Equal(int64(3), acc.GetObservedGeneration())

	irRep := rm.Backends[types.NamespacedName{Namespace: "default", Name: "ir-error"}]
	a.NotNil(irRep)
	irCond := meta.FindStatusCondition(irRep.GetConditions(), "Accepted")
	a.NotNil(irCond)
	a.Equal(metav1.ConditionFalse, irCond.Status)
	a.Equal("Invalid", irCond.Reason)
	a.Equal(`Backend error: "Secret default/foo not found"`, irCond.Message)

	tr := rm.Backends[types.NamespacedName{Namespace: "default", Name: "translation-error"}]
	a.NotNil(tr)
	trCond := meta.FindStatusCondition(tr.GetConditions(), "Accepted")
	a.NotNil(trCond)
	a.Equal(metav1.ConditionFalse, trCond.Status)
	a.Equal("Invalid", trCond.Reason)
	a.Equal(`Backend error: "policy is invalid"`, trCond.Message)
}

func TestGenerateBackendStatusReportIRErrorsTakePrecedence(t *testing.T) {
	a := assert.New(t)

	// a backend with an IR error AND a per-client translation error: the IR error
	// short-circuits translation, so only the IR error should be reported.
	backends := []ir.BackendObjectIR{kgwBackend("both", 1, errors.New("ir boom"))}
	clusters := []uccWithCluster{
		{BackendSource: backendSource("both"), BackendGeneration: 1, Error: errors.New("ir boom")},
	}

	rm := GenerateBackendStatusReport(backends, clusters, nil)
	report := rm.Backends[types.NamespacedName{Namespace: "default", Name: "both"}]
	a.NotNil(report)
	cond := meta.FindStatusCondition(report.GetConditions(), "Accepted")
	a.NotNil(cond)
	a.Equal(`Backend error: "ir boom"`, cond.Message, "IR error reported once, not duplicated")
}

func TestGenerateBackendStatusReportMergesExtraConditions(t *testing.T) {
	a := assert.New(t)

	backends := []ir.BackendObjectIR{kgwBackend("ec2", 7)}
	extraConditions := []ir.BackendObjectStatus{{
		Source: backendSource("ec2"),
		Conditions: []metav1.Condition{{
			Type:    "EndpointsDiscovered",
			Status:  metav1.ConditionTrue,
			Reason:  "Discovered",
			Message: "3 endpoints active",
		}},
	}}

	rm := GenerateBackendStatusReport(backends, nil, extraConditions)
	report := rm.Backends[types.NamespacedName{Namespace: "default", Name: "ec2"}]
	a.NotNil(report)

	// The Accepted condition is still written alongside the contributed condition.
	accepted := meta.FindStatusCondition(report.GetConditions(), "Accepted")
	a.NotNil(accepted)
	a.Equal(metav1.ConditionTrue, accepted.Status)

	discovered := meta.FindStatusCondition(report.GetConditions(), "EndpointsDiscovered")
	a.NotNil(discovered)
	a.Equal(metav1.ConditionTrue, discovered.Status)
	a.Equal("Discovered", discovered.Reason)
	a.Equal("3 endpoints active", discovered.Message)
}
