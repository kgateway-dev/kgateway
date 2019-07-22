package setup

import (
	"context"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

type ClientSet struct {
	// Gateway clients
	V1Gateway       gatewayv1.GatewayClient
	V2alpha1Gateway gatewayv2.GatewayClient
}

func MustClientSet(ctx context.Context) ClientSet {
	// Get shared cache
	kubecfg := mustKubeConfig(ctx)
	kubeCache := kube.NewKubeCache(ctx)

	// Register v1 resource clients
	v1GatewayClientFactory := &factory.KubeResourceClientFactory{
		Crd:             gatewayv1.GatewayCrd,
		Cfg:             kubecfg,
		SharedCache:     kubeCache,
		SkipCrdCreation: false,
	}
	v1GatewayClient, err := gatewayv1.NewGatewayClient(v1GatewayClientFactory)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to set up gatewayv1 gateway client", zap.Error(err))
	}
	if err := v1GatewayClient.Register(); err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to register gatewayv1 gateway client", zap.Error(err))
	}

	// Register v2 resource clients
	v2GatewayClientFactory := &factory.KubeResourceClientFactory{
		Crd:             gatewayv2.GatewayCrd,
		Cfg:             kubecfg,
		SharedCache:     kubeCache,
		SkipCrdCreation: false,
	}
	v2GatewayClient, err := gatewayv2.NewGatewayClient(v2GatewayClientFactory)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to create gatewayv2 gateway client", zap.Error(err))
	}
	if err := v2GatewayClient.Register(); err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to register gatewayv2 gateway client", zap.Error(err))
	}

	return ClientSet{
		V1Gateway:       v1GatewayClient,
		V2alpha1Gateway: v2GatewayClient,
	}
}

func mustKubeConfig(ctx context.Context) *rest.Config {
	kubecfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to get kubernetes config.", zap.Error(err))
	}
	return kubecfg
}
