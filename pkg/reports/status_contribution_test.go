package reports

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func TestStatusContributionsFromReportMapSplitsAndTransfersOwnership(t *testing.T) {
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 7},
	}
	reportMap := NewReportMap()
	reporter := NewReporter(&reportMap)
	reporter.Route(route).ParentRef(&gwv1.ParentReference{Name: "gateway"})

	contributions := StatusContributionsFromReportMap(StatusSource{Kind: GatewayStatusSource, Name: "default/gateway"}, reportMap)
	require.Len(t, contributions, 1)
	contribution := contributions[0]
	require.Equal(t, wellknown.HTTPRouteGVK.GroupKind(), contribution.Target.GroupKind)
	require.Equal(t, types.NamespacedName{Namespace: "default", Name: "route"}, contribution.Target.NamespacedName)
	require.Same(t, reportMap.HTTPRoutes[contribution.Target.NamespacedName], contribution.Route,
		"splitting transfers ownership without cloning every report")
}

// The KRT key and Equals must agree on what identity is. A field compared by Equals but
// absent from the key means two entries KRT treats as the same object compare unequal, and
// recompute forever. StatusTarget used to carry a Version in exactly that position.
func TestStatusContributionKeyCoversEveryFieldEqualsCompares(t *testing.T) {
	nn := types.NamespacedName{Namespace: "default", Name: "route"}
	base := StatusContribution{
		Target: StatusKey{GroupKind: wellknown.HTTPRouteGVK.GroupKind(), NamespacedName: nn},
		Source: StatusSource{Kind: GatewayStatusSource, Name: "default/gw"},
	}

	otherTarget := base
	otherTarget.Target.Kind = "GRPCRoute"
	otherSource := base
	otherSource.Source.Name = "default/other-gw"

	for name, other := range map[string]StatusContribution{
		"different target": otherTarget,
		"different source": otherSource,
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, base.Equals(other))
			require.NotEqual(t, base.ResourceName(), other.ResourceName(),
				"anything Equals distinguishes must also change the KRT key")
		})
	}
}

func TestReduceStatusContributionsMergesPolicyAncestorsAcrossSources(t *testing.T) {
	policy := reporter.PolicyKey{Group: "example.io", Kind: "Policy", Namespace: "default", Name: "policy"}
	gw := ParentRefKey{NamespacedName: types.NamespacedName{Namespace: "default", Name: "gw"}}
	backend := ParentRefKey{NamespacedName: types.NamespacedName{Namespace: "default", Name: "backend"}}

	gatewayReport := NewReportMap()
	gatewayReport.Policies[policy] = &PolicyReport{Ancestors: map[ParentRefKey]*AncestorRefReport{gw: {}}}
	backendReport := NewReportMap()
	backendReport.Policies[policy] = &PolicyReport{Ancestors: map[ParentRefKey]*AncestorRefReport{backend: {}}}

	contributions := append(
		StatusContributionsFromReportMap(StatusSource{Kind: GatewayStatusSource, Name: "default/gw"}, gatewayReport),
		StatusContributionsFromReportMap(StatusSource{Kind: BackendPolicyStatusSource, Name: "default/backend"}, backendReport)...,
	)
	reduced := ReduceStatusContributions(contributions)

	require.Equal(t, map[ParentRefKey]*AncestorRefReport{gw: {}, backend: {}}, reduced.Policy.Ancestors)
}

// Gateway, ListenerSet and Backend reports are whole-owner snapshots, so a second
// contributor's report replaces rather than merges. Which one wins must not depend on the
// order KRT happens to hand us the fragments in, or status would flap between two values
// with nothing changing upstream.
func TestReduceStatusContributionsIsDeterministicForSingleWriterKinds(t *testing.T) {
	target := StatusKey{
		GroupKind:      wellknown.GatewayGVK.GroupKind(),
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "gw"},
	}
	first := StatusContribution{
		Target:       target,
		Source:       StatusSource{Kind: GatewayStatusSource, Name: "default/a"},
		StatusReport: StatusReport{Gateway: &GatewayReport{attachedListenerSets: 1}},
	}
	second := StatusContribution{
		Target:       target,
		Source:       StatusSource{Kind: GatewayStatusSource, Name: "default/b"},
		StatusReport: StatusReport{Gateway: &GatewayReport{attachedListenerSets: 2}},
	}

	forward := ReduceStatusContributions([]StatusContribution{first, second})
	reversed := ReduceStatusContributions([]StatusContribution{second, first})

	require.Equal(t, int32(2), forward.Gateway.attachedListenerSets,
		"the last contribution in source order must win")
	require.Equal(t, forward.Gateway.attachedListenerSets, reversed.Gateway.attachedListenerSets,
		"the winner must not depend on input order")
}

func TestReduceStatusContributionsWarnsOnMultipleSingleWriterContributions(t *testing.T) {
	target := StatusKey{
		GroupKind:      wellknown.BackendGVK.GroupKind(),
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "backend"},
	}
	contribution := func(source string) StatusContribution {
		return StatusContribution{
			Target:       target,
			Source:       StatusSource{Kind: BackendStatusSource, Name: source},
			StatusReport: StatusReport{Backend: &BackendReport{}},
		}
	}

	// The package logger is bound to its own handler at init, so redirecting slog's default
	// would not capture it; swap the package logger itself for the duration of the test.
	var logged bytes.Buffer
	warn := slog.LevelWarn
	restore := logger
	logger = logging.NewWithOptions("reports", logging.Options{Writer: &logged, Level: &warn, Format: logging.TextFormat})
	t.Cleanup(func() { logger = restore })

	ReduceStatusContributions([]StatusContribution{contribution("a")})
	require.Empty(t, logged.String(), "a single writer is the expected topology and must stay quiet")

	ReduceStatusContributions([]StatusContribution{contribution("a"), contribution("b")})
	require.Contains(t, logged.String(), "multiple status contributions for single-writer report kind",
		"a second producer silently discarding the first must be visible")
}

func TestStatusReportEqualsUsesSemanticReportContents(t *testing.T) {
	firstTransition := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	secondTransition := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.True(t, equalityStatusReport(firstTransition).Equals(equalityStatusReport(secondTransition)),
		"LastTransitionTime must not cause KRT churn")

	tests := map[string]func(*StatusReport){
		"gateway scalar": func(report *StatusReport) {
			report.Gateway.attachedListenerSets++
		},
		"condition status": func(report *StatusReport) {
			report.Gateway.conditions[0].Status = metav1.ConditionFalse
		},
		"condition reason": func(report *StatusReport) {
			report.Gateway.conditions[0].Reason = "Different"
		},
		"condition message": func(report *StatusReport) {
			report.Gateway.conditions[0].Message = "different"
		},
		"condition observed generation": func(report *StatusReport) {
			report.Gateway.conditions[0].ObservedGeneration++
		},
		"listener status": func(report *StatusReport) {
			report.ListenerSet.listeners["http"].Status.AttachedRoutes++
		},
		"route parent": func(report *StatusReport) {
			report.Route.Parents[ParentRefKey{NamespacedName: types.NamespacedName{Name: "other"}}] = &ParentRefReport{}
		},
		"policy attachment": func(report *StatusReport) {
			report.Policy.Ancestors[ParentRefKey{NamespacedName: types.NamespacedName{Name: "gateway"}}].AttachmentState = reporter.PolicyAttachmentStateMerged
		},
		"backend generation": func(report *StatusReport) {
			report.Backend.observedGeneration++
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			base := equalityStatusReport(firstTransition)
			changed := equalityStatusReport(firstTransition)
			mutate(&changed)
			require.False(t, base.Equals(changed))
		})
	}
}

func equalityStatusReport(transition time.Time) StatusReport {
	condition := metav1.Condition{
		Type:               "Accepted",
		Status:             metav1.ConditionTrue,
		Reason:             "Accepted",
		Message:            "accepted",
		ObservedGeneration: 1,
		LastTransitionTime: metav1.NewTime(transition),
	}
	parent := ParentRefKey{NamespacedName: types.NamespacedName{Name: "gateway"}}
	return StatusReport{
		Gateway: &GatewayReport{
			conditions:           []metav1.Condition{condition},
			observedGeneration:   1,
			attachedListenerSets: 1,
		},
		ListenerSet: &ListenerSetReport{
			conditions:         []metav1.Condition{condition},
			observedGeneration: 1,
			listeners: map[string]*ListenerReport{
				"http": {Status: gwv1.ListenerStatus{Name: "http", AttachedRoutes: 1, Conditions: []metav1.Condition{condition}}},
			},
		},
		Route: &RouteReport{
			observedGeneration: 1,
			Parents: map[ParentRefKey]*ParentRefReport{
				parent: {Conditions: []metav1.Condition{condition}},
			},
		},
		Policy: &PolicyReport{
			observedGeneration: 1,
			Ancestors: map[ParentRefKey]*AncestorRefReport{
				parent: {Conditions: []metav1.Condition{condition}, AttachmentState: reporter.PolicyAttachmentStateAttached},
			},
		},
		Backend: &BackendReport{
			observedGeneration: 1,
			Conditions:         []metav1.Condition{condition},
		},
	}
}
