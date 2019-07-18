package setup

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/api/v2alpha1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"go.uber.org/zap"
)

type ClientSet struct {
	// Gateway clients
	V1Gateway       v1.GatewayClient
	V2alpha1Gateway v2alpha1.GatewayClient
}

func MustClientSet(ctx context.Context) ClientSet {
	// Get shared cache
	kubecfg := MustKubeConfig(ctx)
	kubeCache := kube.NewKubeCache(ctx)

	// Register v1 resource clients
	v1GatewayClientFactory := &factory.KubeResourceClientFactory{
		Crd:             v1.GatewayCrd,
		Cfg:             kubecfg,
		SharedCache:     kubeCache,
		SkipCrdCreation: false,
	}
	v1GatewayClient, err := v1.NewGatewayClient(v1GatewayClientFactory)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to set up v1 gateway client", zap.Error(err))
	}
	if err := v1GatewayClient.Register(); err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to register v1 gateway client", zap.Error(err))
	}

	// Register v2alpha1 resource clients
	v2alpha1GatewayClientFactory := &factory.KubeResourceClientFactory{
		Crd:             v2alpha1.GatewayCrd,
		Cfg:             kubecfg,
		SharedCache:     kubeCache,
		SkipCrdCreation: false,
	}
	v2alpha1GatewayClient, err := v2alpha1.NewGatewayClient(v2alpha1GatewayClientFactory)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to create v2alpha1 gateway client", zap.Error(err))
	}
	if err := v2alpha1GatewayClient.Register(); err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to register v2alpha1 gateway client", zap.Error(err))
	}

	return ClientSet{
		V1Gateway:       v1GatewayClient,
		V2alpha1Gateway: v2alpha1GatewayClient,
	}
}
