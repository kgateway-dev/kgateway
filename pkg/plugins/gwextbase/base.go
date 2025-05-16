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

func NewTrafficPolicyBuilder(
	ctx context.Context,
	commoncol *common.CommonCollections,
	fetch func(krtctx krt.HandlerContext, extType v1alpha1.GatewayExtensionType) *ir.GatewayExtension,
) *trafficpolicy.TrafficPolicyBuilder {
	return trafficpolicy.NewTrafficPolicyBuilder(ctx, commoncol, fetch)
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
	return trafficpolicy.NewGatewayTranslationPass(ctx, tctx, reporter)
}
