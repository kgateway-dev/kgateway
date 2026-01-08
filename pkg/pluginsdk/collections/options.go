package collections

import (
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type Option func(*option)

type option struct {
	gatewayForDeployerTransformationFunc krtcollections.GatewaysForDeployerTransformationFunction
	gatewayForEnvoyTransformationFunc    krtcollections.GatewaysForEnvoyTransformationFunction
}

func WithGatewayForDeployerTransformationFunc(f func(config *krtcollections.GatewayIndexConfig) func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.GatewayForDeployer) Option {
	return func(o *option) {
		if f != nil {
			o.gatewayForDeployerTransformationFunc = f
		}
	}
}

func WithGatewayForEnvoyTransformationFunc(f func(config *krtcollections.GatewayIndexConfig) func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.Gateway) Option {
	return func(o *option) {
		if f != nil {
			o.gatewayForEnvoyTransformationFunc = f
		}
	}
}
