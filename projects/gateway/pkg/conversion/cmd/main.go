package main

import (
	"context"
	"os"

	"github.com/solo-io/gloo/projects/gateway/pkg/conversion"
	"github.com/solo-io/gloo/projects/gateway/pkg/conversion/setup"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
)

func main() {
	ctx := contextutils.WithLogger(context.Background(), "gateway-conversion")
	clientSet := setup.MustClientSet(ctx)
	gatewayLadder := conversion.NewResourceConverter(
		ctx,
		mustPodNamespace(ctx),
		clientSet.V1Gateway,
		clientSet.V2alpha1Gateway,
		conversion.NewGatewayConverter(),
	)

	if err := gatewayLadder.ConvertAll(); err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to upgrade all existing gateway resources.", zap.Error(err))
	}
}

func mustPodNamespace(ctx context.Context) string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		contextutils.LoggerFrom(ctx).Fatalw("POD_NAMESPACE is not set.")
	}
	return namespace
}
