package httplistenerpolicy

import (
	"context"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/listenerpolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var logger = logging.New("plugin/httplistenerpolicy")

func NewPlugin(ctx context.Context, commoncol *collections.CommonCollections) sdk.Plugin {
	cli := kclient.NewFilteredDelayed[*kgateway.HTTPListenerPolicy](
		commoncol.Client,
		wellknown.HTTPListenerPolicyGVR,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	)
	col := krt.WrapClient(cli, commoncol.KrtOpts.ToOptions("HTTPListenerPolicy")...)
	gk := wellknown.HTTPListenerPolicyGVK.GroupKind()

	policyStatusMarker, policyCol := krt.NewStatusCollection(col, func(krtctx krt.HandlerContext, i *kgateway.HTTPListenerPolicy) (*krtcollections.StatusMarker, *ir.PolicyWrapper) {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: i.Namespace,
			Name:      i.Name,
		}

		// Create status marker if existing status has kgateway controller
		var statusMarker *krtcollections.StatusMarker
		for _, ancestor := range i.Status.Ancestors {
			if string(ancestor.ControllerName) == commoncol.ControllerName {
				statusMarker = &krtcollections.StatusMarker{}
				break
			}
		}
		spec := kgateway.ListenerPolicySpec{
			Default: &kgateway.ListenerConfig{
				HTTPSettings: &i.Spec.HTTPSettings,
			},
		}
		polIr, errs := listenerpolicy.NewListenerPolicyIR(krtctx, commoncol, i.CreationTimestamp.Time, &spec, objSrc)
		polIr.NoOrigin = true
		pol := &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       i,
			PolicyIR:     polIr,
			TargetRefs:   pluginsdkutils.TargetRefsToPolicyRefs(i.Spec.TargetRefs, i.Spec.TargetSelectors),
			Errors:       errs,
		}

		return statusMarker, pol
	}, commoncol.KrtOpts.ToOptions("HTTPListenerPolicyWrapper")...)

	// processMarkers for policies that have existing status but no current report
	processMarkers := func(kctx krt.HandlerContext, reportMap *reports.ReportMap) {
		objStatus := krt.Fetch(kctx, policyStatusMarker)
		for _, status := range objStatus {
			policyKey := reporter.PolicyKey{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: status.Obj.GetNamespace(),
				Name:      status.Obj.GetName(),
			}

			// Add empty status to clear stale status for policies with no valid targets
			if reportMap.Policies[policyKey] == nil {
				rp := reports.NewReporter(reportMap)
				// create empty policy report entry with no ancestor refs
				rp.Policy(policyKey, 0)
			}
		}
	}

	return sdk.Plugin{
		ExtraHasSynced: col.HasSynced,
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			wellknown.HTTPListenerPolicyGVK.GroupKind(): {
				NewGatewayTranslationPass:       NewGatewayTranslationPass,
				Policies:                        policyCol,
				ProcessPolicyStaleStatusMarkers: processMarkers,
				GetPolicyStatus:                 getPolicyStatusFn(cli),
				PatchPolicyStatus:               patchPolicyStatusFn(cli),
				MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
					return policy.MergePolicies(pols, listenerpolicy.MergePolicies, "" /*no merge settings*/)
				},
			},
		},
	}
}

func NewGatewayTranslationPass(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
	return listenerpolicy.NewGatewayTranslationPass(tctx, reporter)
}
