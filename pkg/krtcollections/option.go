package krtcollections

import (
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

type (
	GatewaysForDeployerTransformationFunction func(config *GatewayIndexConfig) func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.GatewayForDeployer
	GatewaysForEnvoyTransformationFunction    func(config *GatewayIndexConfig) func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.Gateway
)

type GatewayIndexConfigOption func(o *GatewayIndexConfig)

func WithGatewayForDeployerTransformationFunc(f GatewaysForDeployerTransformationFunction) GatewayIndexConfigOption {
	return func(o *GatewayIndexConfig) {
		if f != nil {
			o.gatewaysForDeployerTransformationFunc = f
		}
	}
}

func WithGatewayForEnvoyTransformationFunc(f GatewaysForEnvoyTransformationFunction) GatewayIndexConfigOption {
	return func(o *GatewayIndexConfig) {
		if f != nil {
			o.gatewaysForEnvoyTransformationFunc = f
		}
	}
}

func processGatewayIndexConfig(config *GatewayIndexConfig, opts ...GatewayIndexConfigOption) {
	config.byParentRefIndex = krtpkg.UnnamedIndex(config.ListenerSets, func(in *gwxv1a1.XListenerSet) []TargetRefIndexKey {
		pRef := in.Spec.ParentRef
		ns := strOr(pRef.Namespace, "")
		if ns == "" {
			ns = in.GetNamespace()
		}
		// lookup by the root object
		return []TargetRefIndexKey{{
			Group:     wellknown.GatewayGroup,
			Kind:      wellknown.GatewayKind,
			Name:      string(pRef.Name),
			Namespace: ns,
			// this index intentionally doesn't include sectionName
		}}
	})

	config.gatewaysForDeployerTransformationFunc = GatewaysForDeployerTransformationFunc
	config.gatewaysForEnvoyTransformationFunc = GatewaysForEnvoyTransformationFunc

	for _, fn := range opts {
		fn(config)
	}
}
