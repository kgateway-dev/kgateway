package pluginutils

import (
	"context"
	"errors"
	"fmt"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// BuildDesiredPolicyStatusFn builds the desired PolicyStatus for one policy object from
// the merged report map, returning nil when the object has no report (in which case no
// status is written).
type BuildDesiredPolicyStatusFn[T controllers.ComparableObject] func(rm reports.ReportMap, pol T, controllerName string) *gwv1.PolicyStatus

// RegisterPolicyStatus returns a PolicyPlugin.RegisterPolicyStatus hook for a policy CRD
// whose status is a standard gwv1.PolicyStatus. It derives a per-object desired-status
// collection from the merged policy report singleton and registers a writer that merges
// ancestors owned by other controllers at write time.
//
// buildDesired may be nil, in which case the standard ReportMap.BuildPolicyStatus is used.
func RegisterPolicyStatus[T controllers.ComparableObject](
	gvk schema.GroupVersionKind,
	col krt.Collection[T],
	cl kclient.Client[T],
	controllerName string,
	getStatus func(T) gwv1.PolicyStatus,
	build func(om metav1.ObjectMeta, st gwv1.PolicyStatus) T,
	buildDesired BuildDesiredPolicyStatusFn[T],
) func(pluginsdk.PolicyStatusInputs) {
	// Condition-derived error metrics only apply to the standard status shape; custom
	// builders (e.g. BackendTLSPolicy) own their condition semantics.
	defaultBuild := buildDesired == nil
	if buildDesired == nil {
		buildDesired = func(rm reports.ReportMap, pol T, controllerName string) *gwv1.PolicyStatus {
			key := reporter.PolicyKey{
				Group:     gvk.Group,
				Kind:      gvk.Kind,
				Namespace: pol.GetNamespace(),
				Name:      pol.GetName(),
			}
			return rm.BuildPolicyStatus(context.Background(), key, controllerName, getStatus(pol))
		}
	}
	return func(in pluginsdk.PolicyStatusInputs) {
		statuses := krt.NewCollection(col, func(kctx krt.HandlerContext, pol T) *krt.ObjectWithStatus[T, gwv1.PolicyStatus] {
			rw := krt.FetchOne(kctx, in.PolicyReports)
			if rw == nil {
				return nil
			}
			status := buildDesired(rw.Reports(), pol, controllerName)
			if status == nil {
				return nil
			}
			// Normalize through the same merge the writer applies so the desired status is
			// byte-identical to the written result; otherwise ordering differences would
			// re-enqueue (suppressed) writes on every recompute.
			status.Ancestors = statussync.MergePolicyAncestorStatuses(controllerName, getStatus(pol).Ancestors, status.Ancestors)
			return &krt.ObjectWithStatus[T, gwv1.PolicyStatus]{Obj: pol, Status: *status}
		}, in.KrtOpts.ToOptions(gvk.Kind+"Statuses")...)
		statussync.RegisterStatus(in.Collections, gvk, statuses, getStatus)
		in.RegisterWriter(gvk, statussync.Writer[T, gwv1.PolicyStatus]{
			Name:      gvk.Kind,
			Client:    cl,
			Build:     build,
			GetStatus: getStatus,
			Merge: func(current T, desired gwv1.PolicyStatus) gwv1.PolicyStatus {
				desired.Ancestors = statussync.MergePolicyAncestorStatuses(controllerName, getStatus(current).Ancestors, desired.Ancestors)
				return desired
			},
			OnSync: func(res statussync.Resource, current T, status gwv1.PolicyStatus, took time.Duration, err error) {
				statusErr := err
				if defaultBuild {
					statusErr = errors.Join(statusErr, policyStatusConditionError(status))
				}
				statussync.RecordStatusSync(statussync.SyncMetricLabels{
					Name:      gvk.Kind,
					Namespace: res.Namespace,
					Syncer:    "PolicyStatusSyncer",
				}, took, statusErr)
				kmetrics.EndResourceStatusSync(kmetrics.ResourceSyncDetails{
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
// reasons, mirroring the previous status syncer's metrics semantics.
func policyStatusConditionError(status gwv1.PolicyStatus) error {
	for _, ancestor := range status.Ancestors {
		for _, cond := range ancestor.Conditions {
			if cond.Type != string(shared.PolicyConditionAccepted) {
				continue
			}
			if cond.Reason != string(shared.PolicyReasonValid) &&
				cond.Reason != string(shared.PolicyReasonPending) {
				return fmt.Errorf("invalid policy condition")
			}
		}
	}
	return nil
}
