package gwextbase

import (
	"context"

	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/trafficpolicy"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type TrafficPolicy = trafficpolicy.TrafficPolicy
type TrafficPolicyGatewayExtensionIR = trafficpolicy.TrafficPolicyGatewayExtensionIR

func TrafficPolicyBuilder(
	ctx context.Context,
	commoncol *common.CommonCollections, gatewayExtensions krt.Collection[TrafficPolicyGatewayExtensionIR],
) func(krtctx krt.HandlerContext, i *v1alpha1.TrafficPolicy) (*TrafficPolicy, []error) {
	return trafficpolicy.TrafficPolicyBuilder(ctx, commoncol, gatewayExtensions)
}

func TranslateGatewayExtensionBuilder(commoncol *common.CommonCollections) func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR {
	return trafficpolicy.TranslateGatewayExtensionBuilder(commoncol)
}
func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
	return trafficpolicy.NewGatewayTranslationPass(ctx, tctx, reporter)
}
