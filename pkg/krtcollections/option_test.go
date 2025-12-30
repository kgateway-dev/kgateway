package krtcollections

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/util/smallset"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestDefaultGatewayCollectionOptions(t *testing.T) {
	cfg := getConfig(t)
	processGatewayIndexConfig(&cfg)

	assert.NotNil(t, cfg.byParentRefIndex)
	assert.NotNil(t, cfg.gatewaysForDeployerTransformationFunc)
	assert.NotNil(t, cfg.gatewaysForEnvoyTransformationFunc)
}

func TestWithGatewayForDeployerTransformationFunc(t *testing.T) {
	called := false
	customTransformationFunc := func(config *GatewayIndexConfig) func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.GatewayForDeployer {
		called = true
		return func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.GatewayForDeployer {
			return nil
		}
	}
	cfg := getConfig(t)

	NewGatewayIndex(cfg, WithGatewayForDeployerTransformationFunc(customTransformationFunc))
	assert.True(t, called)
}

func TestWithGatewayForEnvoyTransformationFunc(t *testing.T) {
	called := false
	customTransformationFunc := func(config *GatewayIndexConfig) func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.Gateway {
		called = true
		return func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.Gateway {
			return nil
		}
	}
	cfg := getConfig(t)

	NewGatewayIndex(cfg, WithGatewayForEnvoyTransformationFunc(customTransformationFunc))
	assert.True(t, called)
}

func getConfig(t *testing.T) GatewayIndexConfig {
	krtOpts := krtutil.NewKrtOptions(t.Context().Done(), nil)
	return GatewayIndexConfig{
		KrtOpts:             krtOpts,
		ControllerNames:     smallset.New("controller-name"),
		EnvoyControllerName: "envoy-controller-name",
		PolicyIndex:         NewPolicyIndex(krtOpts, nil, settings.Settings{}),
		Gateways:            krt.NewStaticCollection[*gwv1.Gateway](nil, nil, krtOpts.ToOptions("Gateways")...),
		ListenerSets:        krt.NewStaticCollection[*gwxv1a1.XListenerSet](nil, nil, krtOpts.ToOptions("ListenerSets")...),
		GatewayClasses:      krt.NewStaticCollection[*gwv1.GatewayClass](nil, nil, krtOpts.ToOptions("GatewayClasses")...),
		Namespaces:          krt.NewStaticCollection[NamespaceMetadata](nil, nil, krtOpts.ToOptions("Namespaces")...),
	}
}
