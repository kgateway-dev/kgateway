package httplistenerpolicy

import (
	"context"

	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/listenerpolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
)

func NewPlugin(ctx context.Context, commoncol *collections.CommonCollections) sdk.Plugin {
	cli := kclient.NewFilteredDelayed[*kgateway.HTTPListenerPolicy](
		commoncol.Client,
		wellknown.HTTPListenerPolicyGVR,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	)
	col := krt.WrapClient(cli, commoncol.KrtOpts.ToOptions("HTTPListenerPolicy")...)
	gk := wellknown.HTTPListenerPolicyGVK.GroupKind()

	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *kgateway.HTTPListenerPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: i.Namespace,
			Name:      i.Name,
		}

		spec := kgateway.ListenerPolicySpec{
			Default: &kgateway.ListenerDefaultConfig{
				ListenerConfig: kgateway.ListenerConfig{
					HTTPSettings: &i.Spec.HTTPSettings,
				},
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

		return pol
	}, commoncol.KrtOpts.ToOptions("HTTPListenerPolicyWrapper")...)

	return sdk.Plugin{
		ExtraHasSynced: col.HasSynced,
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			wellknown.HTTPListenerPolicyGVK.GroupKind(): {
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
				RegisterPolicyStatus: pluginutils.RegisterPolicyStatus(
					wellknown.HTTPListenerPolicyGVK,
					col,
					cli,
					commoncol.ControllerName,
					func(o *kgateway.HTTPListenerPolicy) gwv1.PolicyStatus { return o.Status },
					func(om metav1.ObjectMeta, st gwv1.PolicyStatus) *kgateway.HTTPListenerPolicy {
						return &kgateway.HTTPListenerPolicy{ObjectMeta: om, Status: st}
					},
					nil,
				),
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
