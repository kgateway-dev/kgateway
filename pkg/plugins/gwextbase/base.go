package gwextbase

import (
	"context"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/trafficpolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

const (
	ExtAuthGlobalDisableFilterName              = trafficpolicy.ExtAuthGlobalDisableFilterName
	ExtAuthGlobalDisableFilterMetadataNamespace = trafficpolicy.ExtAuthGlobalDisableFilterMetadataNamespace
)

type (
	TrafficPolicy                   = trafficpolicy.TrafficPolicy
	TrafficPolicyConstructor        = trafficpolicy.TrafficPolicyConstructor
	ProviderNeededMap               = trafficpolicy.ProviderNeededMap
	TrafficPolicyGatewayExtensionIR = trafficpolicy.TrafficPolicyGatewayExtensionIR
	TrafficPolicyMergeOpts          = trafficpolicy.TrafficPolicyMergeOpts
	TrafficPolicyConstructorOption  = trafficpolicy.TrafficPolicyConstructorOption
)

var (
	ExtAuthzEnabledMetadataMatcher = trafficpolicy.ExtAuthzEnabledMetadataMatcher
	EnableFilterPerRoute           = trafficpolicy.EnableFilterPerRoute
	MergeTrafficPolicies           = trafficpolicy.MergeTrafficPolicies
	AddDisableFilterIfNeeded       = trafficpolicy.AddDisableFilterIfNeeded

	// WithSourceGroupKind sets the identity a ReferenceGrant has to name for the
	// cross-namespace references held by a TrafficPolicySpec. See
	// trafficpolicy.WithSourceGroupKind.
	WithSourceGroupKind = trafficpolicy.WithSourceGroupKind
)

// NewTrafficPolicyConstructor creates a traffic policy constructor. This converts a traffic policy into its IR form.
func NewTrafficPolicyConstructor(
	ctx context.Context,
	commoncol *collections.CommonCollections,
	opts ...trafficpolicy.TrafficPolicyConstructorOption,
) *trafficpolicy.TrafficPolicyConstructor {
	return trafficpolicy.NewTrafficPolicyConstructor(ctx, commoncol, opts...)
}

func NewGatewayTranslationPass(tctx ir.GwTranslationCtx, reporter reporter.Reporter, enableAuthSucceededMetadata bool) ir.ProxyTranslationPass {
	return trafficpolicy.NewGatewayTranslationPass(tctx, reporter, enableAuthSucceededMetadata)
}
