package pluginutils

import (
	"errors"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// BuildDesiredPolicyStatusFn builds the desired PolicyStatus for one policy object from
// its typed report fragment, returning nil when the object has no report.
type BuildDesiredPolicyStatusFn[T controllers.ComparableObject] func(report *reports.PolicyReport, pol T, controllerName string) *gwv1.PolicyStatus

// ConditionErrorMetric selects how a policy writer reports its status-sync metric: whether
// the conditions it publishes are graded against the standard policy Accepted reasons
// (Valid, Pending), or only write failures count as errors.
//
// It is a separate, explicit choice rather than something inferred from which builder a
// plugin supplied. Those are independent questions — a plugin can build its own status and
// still use the standard condition vocabulary — and inferring one from the other meant such
// a plugin silently lost the metric with nothing to indicate it.
type ConditionErrorMetric bool

const (
	// StandardConditionErrorMetric grades published conditions against shared.PolicyReasonValid
	// and shared.PolicyReasonPending. Use it for any status built from the standard policy
	// condition vocabulary.
	StandardConditionErrorMetric ConditionErrorMetric = true
	// NoConditionErrorMetric reports only write failures. Use it for a status whose conditions
	// come from a different vocabulary — e.g. BackendTLSPolicy, which reports the Gateway API's
	// own PolicyReasonAccepted and would otherwise be graded as permanently failing.
	NoConditionErrorMetric ConditionErrorMetric = false
)

// RegisterPolicyStatus returns a PolicyPlugin.RegisterPolicyStatus hook for a policy CRD
// whose status is a standard gwv1.PolicyStatus, built by the standard typed policy status
// builder. It derives a per-object desired-status source and registers a writer that builds
// from the latest merged policy report, merging ancestors owned by other controllers at
// write time.
//
// Use RegisterPolicyStatusWithBuilder for a policy that builds its own desired status.
func RegisterPolicyStatus[T controllers.ComparableObject](
	gvk schema.GroupVersionKind,
	col krt.Collection[T],
	cl kclient.Client[T],
	controllerName string,
	getStatus func(T) gwv1.PolicyStatus,
	build func(om metav1.ObjectMeta, st gwv1.PolicyStatus) T,
) func(pluginsdk.PolicyStatusInputs) {
	return RegisterPolicyStatusWithBuilder(gvk, col, cl, controllerName, getStatus, build, nil, StandardConditionErrorMetric)
}

// RegisterPolicyStatusWithBuilder is RegisterPolicyStatus for a policy CRD that builds its
// own desired status, such as BackendTLSPolicy. conditionMetric says explicitly whether that
// status uses the standard condition vocabulary; see ConditionErrorMetric.
//
// A nil buildDesired selects the standard builder, which is what RegisterPolicyStatus is
// for — call that instead.
func RegisterPolicyStatusWithBuilder[T controllers.ComparableObject](
	gvk schema.GroupVersionKind,
	col krt.Collection[T],
	cl kclient.Client[T],
	controllerName string,
	getStatus func(T) gwv1.PolicyStatus,
	build func(om metav1.ObjectMeta, st gwv1.PolicyStatus) T,
	buildDesired BuildDesiredPolicyStatusFn[T],
	conditionMetric ConditionErrorMetric,
) func(pluginsdk.PolicyStatusInputs) {
	return func(in pluginsdk.PolicyStatusInputs) {
		desiredFor := buildDesired
		if desiredFor == nil {
			desiredFor = func(report *reports.PolicyReport, pol T, controllerName string) *gwv1.PolicyStatus {
				key := reporter.PolicyKey{
					Group:     gvk.Group,
					Kind:      gvk.Kind,
					Namespace: pol.GetNamespace(),
					Name:      pol.GetName(),
				}
				return reports.BuildPolicyStatus(report, key, controllerName, getStatus(pol))
			}
		}
		statusReports := statussync.RegisterKind(
			in.Collections, gvk, col,
			in.StatusContributions, in.ContributionsByTarget,
			in.KrtOpts.ToOptions(gvk.Kind+"StatusReports")...,
		)
		in.RegisterWriter(gvk, statussync.Writer[T, gwv1.PolicyStatus]{
			Name: gvk.Kind,
			// Read from the collection that enqueues this policy, not from cl: cl is a
			// delayed client whose Get returns nil until its own informer loads.
			Current: statussync.CollectionSource(col),
			Desired: func(pol T) (gwv1.PolicyStatus, bool) {
				nn := types.NamespacedName{Namespace: pol.GetNamespace(), Name: pol.GetName()}
				report, ok := statussync.ReportFor(statusReports, gvk, nn)
				if !ok {
					return gwv1.PolicyStatus{}, false
				}
				status := desiredFor(report.Policy, pol, controllerName)
				if status == nil {
					// Merge will clear only ancestors owned by this controller, which is worth
					// a write only if we have ancestors to clear. Every policy of this kind
					// lands here, including ones no Gateway we translate ever attached.
					if !statussync.OwnsAnyPolicyAncestor(controllerName, getStatus(pol).Ancestors) {
						return gwv1.PolicyStatus{}, false
					}
					return gwv1.PolicyStatus{}, true
				}
				return *status, true
			},
			UpdateStatus: statussync.ClientWriter(cl, build),
			GetStatus:    getStatus,
			Merge: func(current T, desired gwv1.PolicyStatus) gwv1.PolicyStatus {
				desired.Ancestors = statussync.MergePolicyAncestorStatuses(controllerName, getStatus(current).Ancestors, desired.Ancestors)
				return desired
			},
			OnSync: func(res statussync.Resource, current T, status gwv1.PolicyStatus, took time.Duration, err error) {
				statusErr := err
				if conditionMetric {
					statusErr = errors.Join(statusErr, policyStatusConditionError(status, controllerName))
				}
				statussync.RecordStatusSync(statussync.SyncMetricLabels{
					Name:      gvk.Kind,
					Namespace: res.Namespace,
					Syncer:    "PolicyStatusSyncer",
				}, took, statusErr)
				statussync.EndResourceStatusSyncOnWriteSuccess(err, kmetrics.ResourceSyncDetails{
					Namespace:    res.Namespace,
					Gateway:      "",
					ResourceType: gvk.Kind,
					ResourceName: res.Name,
				})
			},
		})
	}
}

// policyStatusConditionError derives an error from invalid policy Accepted condition
// reasons, mirroring the previous status syncer's metrics semantics. status is the merged
// status, so ancestors owned by other controllers are skipped: their conditions are not
// ours to report on.
func policyStatusConditionError(status gwv1.PolicyStatus, controllerName string) error {
	for _, ancestor := range status.Ancestors {
		if string(ancestor.ControllerName) != controllerName {
			continue
		}
		if err := statussync.ConditionError(nil, ancestor.Conditions,
			policyConditionTypes, policyAcceptedReasons, "invalid policy condition"); err != nil {
			return err
		}
	}
	return nil
}

var (
	policyConditionTypes  = []string{string(shared.PolicyConditionAccepted)}
	policyAcceptedReasons = []string{string(shared.PolicyReasonValid), string(shared.PolicyReasonPending)}
)
