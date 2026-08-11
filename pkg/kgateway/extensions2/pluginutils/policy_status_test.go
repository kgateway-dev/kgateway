package pluginutils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

const (
	testController  = "kgateway.dev/kgateway"
	otherController = "other.example/controller"
	testNamespace   = "default"
	testPolicyName  = "policy"
)

// BackendTLSPolicy stands in for any policy CRD with a standard gwv1.PolicyStatus; the hook
// under test is generic over the policy type.
var policyGVK = schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "BackendTLSPolicy"}

func policyResource() statussync.Resource {
	return statussync.Resource{
		GroupVersionKind: policyGVK,
		NamespacedName:   types.NamespacedName{Namespace: testNamespace, Name: testPolicyName},
	}
}

// policyReport builds a contribution carrying one ancestor for our controller, with the
// supplied Accepted reason.
func policyReport(acceptedReason shared.PolicyConditionReason) reports.StatusContribution {
	reportMap := reports.NewReportMap()
	key := reporter.PolicyKey{
		Group:     policyGVK.Group,
		Kind:      policyGVK.Kind,
		Namespace: testNamespace,
		Name:      testPolicyName,
	}
	reports.NewReporter(&reportMap).Policy(key, 1).
		AncestorRef(gwv1.ParentReference{Name: "gw"}).
		SetCondition(reporter.PolicyCondition{
			Type:   string(shared.PolicyConditionAccepted),
			Status: metav1.ConditionTrue,
			Reason: string(acceptedReason),
		})

	contributions := reports.StatusContributionsFromReportMap(
		reports.StatusSource{Kind: reports.GatewayStatusSource, Name: testNamespace + "/gw"}, reportMap)
	return contributions[0]
}

type policyStatusFixture struct {
	writer statussync.ResourceStatusSyncer
	// current reads the policy back from the fake API server.
	current func(t *testing.T) *gwv1.BackendTLSPolicy
	// statusUpdates counts status subresource writes that actually reached the API server.
	statusUpdates func() int
}

// newPolicyStatusFixture registers a policy status hook against a fake API server holding
// one policy, and returns the writer it registered. A nil buildDesired selects the standard
// builder (RegisterPolicyStatus); anything else goes through RegisterPolicyStatusWithBuilder.
// The metric choice is always stated: a custom builder no longer implies anything about it.
func newPolicyStatusFixture(
	t *testing.T,
	existing *gwv1.BackendTLSPolicy,
	contributions []reports.StatusContribution,
	buildDesired BuildDesiredPolicyStatusFn[*gwv1.BackendTLSPolicy],
	conditionMetric ConditionErrorMetric,
) policyStatusFixture {
	t.Helper()
	stop := test.NewStop(t)
	c := kube.NewFakeClient()
	cl := kclient.NewFiltered[*gwv1.BackendTLSPolicy](c, kclient.Filter{})
	policies := krt.WrapClient(cl, krt.WithStop(stop))

	_, err := c.GatewayAPI().GatewayV1().BackendTLSPolicies(testNamespace).
		Create(context.Background(), existing, metav1.CreateOptions{})
	require.NoError(t, err)
	c.RunAndWait(stop)

	contributionCol := krt.NewStaticCollection(nil, contributions, krt.WithStop(stop))
	byTarget := krtpkg.UnnamedIndex(contributionCol, func(c reports.StatusContribution) []reports.StatusKey {
		return []reports.StatusKey{c.Target}
	})

	var registered statussync.ResourceStatusSyncer
	collections := statussync.NewStatusCollections()
	getStatus := func(p *gwv1.BackendTLSPolicy) gwv1.PolicyStatus { return p.Status }
	buildObject := func(om metav1.ObjectMeta, st gwv1.PolicyStatus) *gwv1.BackendTLSPolicy {
		return &gwv1.BackendTLSPolicy{ObjectMeta: om, Status: st}
	}
	register := RegisterPolicyStatusWithBuilder(policyGVK, policies, cl, testController,
		getStatus, buildObject, buildDesired, conditionMetric)
	if buildDesired == nil && conditionMetric == StandardConditionErrorMetric {
		// The arguments a standard policy plugin passes, so go through the entry point it calls.
		register = RegisterPolicyStatus(policyGVK, policies, cl, testController, getStatus, buildObject)
	}
	register(pluginsdk.PolicyStatusInputs{
		Collections:           collections,
		StatusContributions:   contributionCol,
		ContributionsByTarget: byTarget,
		RegisterWriter: func(_ schema.GroupVersionKind, syncer statussync.ResourceStatusSyncer) {
			registered = syncer
		},
	})
	require.NotNil(t, registered, "the hook must register a writer")

	require.Eventually(t, func() bool {
		return cl.Get(testPolicyName, testNamespace) != nil
	}, 5*time.Second, 10*time.Millisecond, "informer should observe the policy")
	// The writer builds desired status from the reducer, so it must have reduced the
	// contributions before ApplyStatus runs. RegisterResourceReports enrols the reducer in
	// this barrier, which is exactly the tracking the hook is responsible for.
	require.Eventually(t, collections.HasSynced, 5*time.Second, 10*time.Millisecond,
		"the report reducer must be registered in the cache sync barrier and synced")

	fake := c.GatewayAPI().(*gatewayfake.Clientset)
	return policyStatusFixture{
		writer: registered,
		current: func(t *testing.T) *gwv1.BackendTLSPolicy {
			t.Helper()
			got, err := c.GatewayAPI().GatewayV1().BackendTLSPolicies(testNamespace).
				Get(context.Background(), testPolicyName, metav1.GetOptions{})
			require.NoError(t, err)
			return got
		},
		statusUpdates: func() int {
			n := 0
			for _, a := range fake.Actions() {
				if a.GetVerb() == "update" && a.GetSubresource() == "status" {
					n++
				}
			}
			return n
		},
	}
}

func emptyPolicy() *gwv1.BackendTLSPolicy {
	return &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: testPolicyName, Namespace: testNamespace, ResourceVersion: "1"},
	}
}

// RegisterPolicyStatus uses the standard typed policy status builder.
func TestRegisterPolicyStatusUsesTheStandardBuilder(t *testing.T) {
	f := newPolicyStatusFixture(t, emptyPolicy(),
		[]reports.StatusContribution{policyReport(shared.PolicyReasonValid)}, nil, StandardConditionErrorMetric)

	f.writer.ApplyStatus(context.Background(), policyResource())

	status := f.current(t).Status
	require.Len(t, status.Ancestors, 1)
	require.Equal(t, gwv1.GatewayController(testController), status.Ancestors[0].ControllerName)
	require.Equal(t, gwv1.ObjectName("gw"), status.Ancestors[0].AncestorRef.Name)
}

// A supplied buildDesired must be used verbatim: plugins like BackendTLSPolicy own their
// condition semantics and the default builder would produce a different status.
func TestRegisterPolicyStatusUsesSuppliedBuilder(t *testing.T) {
	called := 0
	custom := func(_ *reports.PolicyReport, _ *gwv1.BackendTLSPolicy, controllerName string) *gwv1.PolicyStatus {
		called++
		return &gwv1.PolicyStatus{Ancestors: []gwv1.PolicyAncestorStatus{{
			AncestorRef:    gwv1.ParentReference{Name: "custom"},
			ControllerName: gwv1.GatewayController(controllerName),
		}}}
	}

	f := newPolicyStatusFixture(t, emptyPolicy(),
		[]reports.StatusContribution{policyReport(shared.PolicyReasonValid)}, custom, NoConditionErrorMetric)

	f.writer.ApplyStatus(context.Background(), policyResource())

	require.Positive(t, called, "the supplied builder must be used instead of the default")
	status := f.current(t).Status
	require.Len(t, status.Ancestors, 1)
	require.Equal(t, gwv1.ObjectName("custom"), status.Ancestors[0].AncestorRef.Name)
}

// Policy writers must converge like every other writer: the status they write echoes back
// through the collection they read, and only the live-vs-desired skip stops the cycle.
// Plugins hand their writer to the pipeline as a statussync.ResourceStatusSyncer, so this is
// also the pattern a downstream registration uses to run the shared harness on its own
// writer: recover the typed Writer and hand it an apply func.
func TestRegisterPolicyStatusWriterIsIdempotent(t *testing.T) {
	stale := metav1.NewTime(time.Now().Add(-time.Hour))
	existing := emptyPolicy()
	existing.Status = gwv1.PolicyStatus{Ancestors: []gwv1.PolicyAncestorStatus{
		// Another controller's ancestors, stored in the reverse of the order our merge
		// canonicalizes to: publishing reorders them once and must then be stable.
		{
			AncestorRef:    gwv1.ParentReference{Name: "zzz-their-gw"},
			ControllerName: gwv1.GatewayController(otherController),
		},
		{
			AncestorRef:    gwv1.ParentReference{Name: "aaa-their-gw"},
			ControllerName: gwv1.GatewayController(otherController),
		},
		{
			AncestorRef:    gwv1.ParentReference{Name: "gw"},
			ControllerName: gwv1.GatewayController(testController),
			Conditions: []metav1.Condition{{
				Type:               string(shared.PolicyConditionAccepted),
				Status:             metav1.ConditionFalse,
				Reason:             string(shared.PolicyReasonPending),
				LastTransitionTime: stale,
			}},
		},
	}}

	f := newPolicyStatusFixture(t, existing,
		[]reports.StatusContribution{policyReport(shared.PolicyReasonValid)}, nil, StandardConditionErrorMetric)

	writer, ok := f.writer.(statussync.Writer[*gwv1.BackendTLSPolicy, gwv1.PolicyStatus])
	require.True(t, ok, "the registered syncer should be the generic writer")
	require.True(t, statussync.WriterWouldWrite(writer, f.current(t)),
		"the stale status must actually be written, or the check below proves nothing")
	require.NoError(t, statussync.CheckWriterIdempotent(writer, f.current(t),
		func(current *gwv1.BackendTLSPolicy, status gwv1.PolicyStatus) *gwv1.BackendTLSPolicy {
			next := current.DeepCopy()
			next.Status = *status.DeepCopy()
			return next
		}))
}

// A policy that produced no report at all (it was not translated) must have its own stale
// ancestors cleared without disturbing ancestors owned by other controllers.
func TestRegisterPolicyStatusWithNilReportClearsOnlyOurAncestors(t *testing.T) {
	existing := emptyPolicy()
	existing.Status = gwv1.PolicyStatus{Ancestors: []gwv1.PolicyAncestorStatus{
		{
			AncestorRef:    gwv1.ParentReference{Name: "stale-gw"},
			ControllerName: gwv1.GatewayController(testController),
		},
		{
			AncestorRef:    gwv1.ParentReference{Name: "their-gw"},
			ControllerName: gwv1.GatewayController(otherController),
		},
	}}

	// A contribution exists for the policy (so the reducer has an entry to key on) but it
	// carries no policy report, which is what an untranslated policy looks like.
	emptyContribution := reports.StatusContribution{
		Target: reports.StatusKey{
			GroupKind:      policyGVK.GroupKind(),
			NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testPolicyName},
		},
		Source: reports.StatusSource{Kind: reports.GatewayStatusSource, Name: testNamespace + "/gw"},
	}
	f := newPolicyStatusFixture(t, existing, []reports.StatusContribution{emptyContribution}, nil, StandardConditionErrorMetric)

	f.writer.ApplyStatus(context.Background(), policyResource())

	status := f.current(t).Status
	require.Len(t, status.Ancestors, 1, "our stale ancestor must be cleared")
	require.Equal(t, gwv1.GatewayController(otherController), status.Ancestors[0].ControllerName,
		"another controller's ancestor must survive")
}

func TestRegisterPolicyStatusDefaultBuilderRecordsConditionErrors(t *testing.T) {
	tests := map[string]struct {
		reason     shared.PolicyConditionReason
		wantResult string
	}{
		"valid":   {reason: shared.PolicyReasonValid, wantResult: "success"},
		"pending": {reason: shared.PolicyReasonPending, wantResult: "success"},
		"invalid": {reason: shared.PolicyReasonInvalid, wantResult: "error"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			statussync.ResetMetrics()
			kmetrics.ResetMetrics()

			f := newPolicyStatusFixture(t, emptyPolicy(),
				[]reports.StatusContribution{policyReport(tc.reason)}, nil, StandardConditionErrorMetric)
			f.writer.ApplyStatus(context.Background(), policyResource())

			gathered := metricstest.MustGatherMetrics(t)
			gathered.AssertMetricsInclude("kgateway_status_syncer_status_syncs_total", []metricstest.ExpectMetric{
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "name", Value: policyGVK.Kind},
						{Name: "namespace", Value: testNamespace},
						{Name: "result", Value: tc.wantResult},
						{Name: "syncer", Value: "PolicyStatusSyncer"},
					},
					Value: 1,
				},
			})
		})
	}
}

// Whether conditions are graded against the standard Accepted reasons is the caller's
// explicit choice, independent of which builder it supplies. It used to be inferred from
// "buildDesired == nil", which silently dropped the metric for a plugin that built its own
// standard-shaped status — the failure this pair of cases pins down.
func TestRegisterPolicyStatusGradesConditionsWhenAsked(t *testing.T) {
	// A custom builder publishing an invalid standard condition. Both cases below use it, so
	// the only difference between them is the ConditionErrorMetric argument.
	custom := func(*reports.PolicyReport, *gwv1.BackendTLSPolicy, string) *gwv1.PolicyStatus {
		return &gwv1.PolicyStatus{Ancestors: []gwv1.PolicyAncestorStatus{{
			AncestorRef:    gwv1.ParentReference{Name: "gw"},
			ControllerName: gwv1.GatewayController(testController),
			Conditions: []metav1.Condition{{
				Type:   string(shared.PolicyConditionAccepted),
				Status: metav1.ConditionFalse,
				Reason: string(shared.PolicyReasonInvalid),
			}},
		}}}
	}

	tests := map[string]struct {
		metric     ConditionErrorMetric
		wantResult string
	}{
		"graded":     {metric: StandardConditionErrorMetric, wantResult: "error"},
		"not graded": {metric: NoConditionErrorMetric, wantResult: "success"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			statussync.ResetMetrics()
			kmetrics.ResetMetrics()

			f := newPolicyStatusFixture(t, emptyPolicy(),
				[]reports.StatusContribution{policyReport(shared.PolicyReasonValid)}, custom, tc.metric)
			f.writer.ApplyStatus(context.Background(), policyResource())

			gathered := metricstest.MustGatherMetrics(t)
			gathered.AssertMetricsInclude("kgateway_status_syncer_status_syncs_total", []metricstest.ExpectMetric{
				&metricstest.ExpectedMetric{
					Labels: []metrics.Label{
						{Name: "name", Value: policyGVK.Kind},
						{Name: "namespace", Value: testNamespace},
						{Name: "result", Value: tc.wantResult},
						{Name: "syncer", Value: "PolicyStatusSyncer"},
					},
					Value: 1,
				},
			})
		})
	}
}

// A policy no Gateway we translate ever attached still reaches the writer: the report
// reducer holds an entry for every raw policy, so Desired is asked about all of them. It
// must decline rather than publish an empty status, which the merge would turn into a
// non-nil empty ancestor list and the no-op check would see as a change — one spurious
// write per unrelated policy, on every leadership acquisition.
func TestRegisterPolicyStatusSkipsPoliciesWeDoNotOwn(t *testing.T) {
	tests := map[string]gwv1.PolicyStatus{
		"never written by anyone": {},
		// Stored in the reverse of the order our merge canonicalizes to, so a write would
		// reorder another controller's ancestors -- a rewrite war against any peer that
		// compares its own list order-sensitively.
		"owned entirely by another controller, unsorted": {Ancestors: []gwv1.PolicyAncestorStatus{
			{
				AncestorRef:    gwv1.ParentReference{Name: "zzz-their-gw"},
				ControllerName: gwv1.GatewayController(otherController),
			},
			{
				AncestorRef:    gwv1.ParentReference{Name: "aaa-their-gw"},
				ControllerName: gwv1.GatewayController(otherController),
			},
		}},
	}

	for name, existingStatus := range tests {
		t.Run(name, func(t *testing.T) {
			existing := emptyPolicy()
			existing.Status = existingStatus
			f := newPolicyStatusFixture(t, existing, nil, nil, StandardConditionErrorMetric)

			f.writer.ApplyStatus(context.Background(), policyResource())

			require.Zero(t, f.statusUpdates(), "a policy we own no ancestor on must not be written")
			require.Equal(t, existingStatus.Ancestors, f.current(t).Status.Ancestors,
				"another controller's ancestors must be left exactly as they are")
		})
	}
}
