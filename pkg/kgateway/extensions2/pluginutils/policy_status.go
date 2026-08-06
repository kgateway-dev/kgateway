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

// RegisterPolicyStatus returns a PolicyPlugin.RegisterPolicyStatus hook for a policy CRD
// whose status is a standard gwv1.PolicyStatus. It derives a per-object desired-status
// source and registers a writer that builds from the latest merged policy report, merging
// ancestors owned by other controllers at write time.
//
// buildDesired may be nil, in which case the standard typed policy status builder is used.
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
	return func(in pluginsdk.PolicyStatusInputs) {
		desiredFor := buildDesired
		if desiredFor == nil {
			// The builder outlives this call, so capture the controller's root context
			// rather than a request-scoped one.
			ctx := in.Ctx
			if ctx == nil {
				ctx = context.Background()
			}
			desiredFor = func(report *reports.PolicyReport, pol T, controllerName string) *gwv1.PolicyStatus {
				key := reporter.PolicyKey{
					Group:     gvk.Group,
					Kind:      gvk.Kind,
					Namespace: pol.GetNamespace(),
					Name:      pol.GetName(),
				}
				return reports.BuildPolicyStatus(ctx, report, key, controllerName, getStatus(pol))
			}
		}
		statusReports := statussync.NewResourceReports(
			col,
			in.StatusContributions,
			in.ContributionsByTarget,
			func(pol T) statussync.Resource {
				return statussync.Resource{
					GroupVersionKind: gvk,
					NamespacedName:   types.NamespacedName{Namespace: pol.GetNamespace(), Name: pol.GetName()},
				}
			},
			in.KrtOpts.ToOptions(gvk.Kind+"StatusReports")...,
		)
		statussync.RegisterResource(in.Collections, gvk, col)
		statussync.RegisterResourceReports(in.Collections, statusReports)
		in.RegisterWriter(gvk, statussync.Writer[T, gwv1.PolicyStatus]{
			Name:   gvk.Kind,
			Client: cl,
			Desired: func(pol T) (gwv1.PolicyStatus, bool) {
				target := reports.StatusKey{
					GroupKind:      gvk.GroupKind(),
					NamespacedName: types.NamespacedName{Namespace: pol.GetNamespace(), Name: pol.GetName()},
				}
				rw := statusReports.GetKey(target.String())
				if rw == nil {
					return gwv1.PolicyStatus{}, false
				}
				status := desiredFor(rw.Report.Policy, pol, controllerName)
				if status == nil {
					// Merge will clear only ancestors owned by this controller.
					return gwv1.PolicyStatus{}, true
				}
				return *status, true
			},
			Build:     build,
			GetStatus: getStatus,
			NotReady:  in.NotReady,
			Merge: func(current T, desired gwv1.PolicyStatus) gwv1.PolicyStatus {
				desired.Ancestors = statussync.MergePolicyAncestorStatuses(controllerName, getStatus(current).Ancestors, desired.Ancestors)
				return desired
			},
			OnSync: func(res statussync.Resource, current T, status gwv1.PolicyStatus, took time.Duration, err error) {
				statusErr := err
				if defaultBuild {
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
