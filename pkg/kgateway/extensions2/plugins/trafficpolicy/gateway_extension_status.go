package trafficpolicy

import (
	"time"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// registerGatewayExtensionStatus publishes the Accepted condition on the GatewayExtension
// resource itself, mirroring Backend: True/Accepted when the extension translated, and
// False/Invalid with the translation error when it did not.
//
// A GatewayExtension's translation result is produced by this plugin (see
// TranslateGatewayExtensionBuilder), which stores the error on
// TrafficPolicyGatewayExtensionIR.Err instead of surfacing it anywhere. Reporting it on the
// resource requires the IR collection, so this registration lives here rather than in the
// proxy syncer's built-in status kinds.
//
// It is self-contained: the contributions, the report reducer, and the status writer for
// GatewayExtension are all built and registered here, rather than joined into the syncer's
// shared status-contribution collection. The syncer still owns the write queue, so status
// is only written while this replica is leader.
func registerGatewayExtensionStatus(
	in sdk.PolicyStatusInputs,
	commoncol *collections.CommonCollections,
	gatewayExtensions krt.Collection[TrafficPolicyGatewayExtensionIR],
) {
	rawExts := commoncol.RawGatewayExtensions
	if rawExts == nil {
		return
	}
	gvk := wellknown.GatewayExtensionGVK

	// One contribution per GatewayExtension, keyed on the extension's own identity. The
	// condition is rebuilt whenever the raw object or its translation changes, because the
	// fetch on gatewayExtensions registers that dependency.
	contributions := krt.NewCollection(rawExts, func(kctx krt.HandlerContext, ext *kgateway.GatewayExtension) *reports.StatusContribution {
		var translatedErr error
		translated := krt.FetchOne(kctx, gatewayExtensions, krt.FilterObjectName(types.NamespacedName{
			Namespace: ext.Namespace,
			Name:      ext.Name,
		}))
		if translated != nil {
			translatedErr = translated.Err
		}
		return gatewayExtensionStatusContribution(ext, translatedErr)
	}, commoncol.KrtOpts.ToOptions("GatewayExtensionStatusContributions")...)

	byTarget := krtpkg.UnnamedIndex(contributions, func(c reports.StatusContribution) []reports.StatusKey {
		return []reports.StatusKey{c.Target}
	})
	extReports := statussync.RegisterKind(in.Collections, gvk, rawExts, contributions, byTarget,
		commoncol.KrtOpts.ToOptions("GatewayExtensionStatusReports")...)

	// The write-side client is separate from the collection's informer: the collection
	// enqueues the resource, and only the write goes through this client.
	cl := kclient.NewFilteredDelayed[*kgateway.GatewayExtension](
		commoncol.Client,
		wellknown.GatewayExtensionGVR,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	)
	in.RegisterWriter(gvk, statussync.Writer[*kgateway.GatewayExtension, kgateway.GatewayExtensionStatus]{
		Name:    "gatewayExtension",
		Current: statussync.CollectionSource(rawExts),
		Desired: func(res statussync.Resource, current *kgateway.GatewayExtension) (kgateway.GatewayExtensionStatus, bool) {
			report, ok := statussync.ReportFor(extReports, res)
			if !ok {
				return kgateway.GatewayExtensionStatus{}, false
			}
			status := reports.BuildGatewayExtensionStatus(report.GatewayExtension, current.Status)
			if status == nil {
				return kgateway.GatewayExtensionStatus{}, false
			}
			return *status, true
		},
		UpdateStatus: statussync.ClientWriter(cl, func(om metav1.ObjectMeta, st kgateway.GatewayExtensionStatus) *kgateway.GatewayExtension {
			return &kgateway.GatewayExtension{ObjectMeta: om, Status: st}
		}),
		GetStatus: func(o *kgateway.GatewayExtension) kgateway.GatewayExtensionStatus { return o.Status },
		OnSync: func(res statussync.Resource, _ *kgateway.GatewayExtension, _ kgateway.GatewayExtensionStatus, took time.Duration, err error) {
			statussync.RecordStatusSyncOutcome(
				statussync.SyncMetricLabels{
					Name:      res.Name,
					Namespace: res.Namespace,
					Syncer:    "GatewayExtensionStatusSyncer",
				},
				kmetrics.ResourceSyncDetails{
					Namespace:    res.Namespace,
					ResourceType: gvk.Kind,
					ResourceName: res.Name,
				},
				took, err, err)
		},
	})
}

// gatewayExtensionStatusContribution is the pure mapping from one GatewayExtension and its
// translation error to the single status contribution that reports it: True/Accepted when
// translatedErr is nil, False/Invalid carrying the error otherwise.
func gatewayExtensionStatusContribution(
	ext *kgateway.GatewayExtension,
	translatedErr error,
) *reports.StatusContribution {
	var errs []error
	if translatedErr != nil {
		errs = append(errs, translatedErr)
	}
	cond := pluginutils.BuildGatewayExtensionCondition("GatewayExtension", errs)
	return &reports.StatusContribution{
		Target: reports.StatusKey{
			GroupKind:      wellknown.GatewayExtensionGVK.GroupKind(),
			NamespacedName: types.NamespacedName{Namespace: ext.Namespace, Name: ext.Name},
		},
		Source: reports.StatusSource{
			Kind: reports.GatewayExtensionStatusSource,
			Name: ext.Namespace + "/" + ext.Name,
		},
		StatusReport: reports.StatusReport{
			GatewayExtension: reports.NewGatewayExtensionReport(ext.Generation, []metav1.Condition{cond}),
		},
	}
}
