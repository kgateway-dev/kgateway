package pluginutils

import (
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

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
	return RegisterPolicyStatusWithBuilder(gvk, col, cl, controllerName, getStatus, build, nil)
}

// RegisterPolicyStatusWithBuilder is RegisterPolicyStatus for a policy CRD that builds its
// own desired status, such as BackendTLSPolicy.
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
			Desired: func(res statussync.Resource, pol T) (gwv1.PolicyStatus, bool) {
				report, ok := statussync.ReportFor(statusReports, res)
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
			OnSync: func(res statussync.Resource, _ T, _ gwv1.PolicyStatus, err error) {
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
