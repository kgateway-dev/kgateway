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
	// Set v1 to served for the duration of the job.
	crd := setup.MustSetV1ToServed(ctx)
	defer setup.MustSetV1ToNotServed(ctx, crd)

	clientSet := setup.MustClientSet(ctx)
	gatewayLadder := conversion.NewLadder(
		ctx,
		mustPodNamespace(ctx),
		clientSet.V1Gateway,
		clientSet.V2alpha1Gateway,
		conversion.NewGatewayConverter(),
	)

	if err := gatewayLadder.Climb(); err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to upgrade existing gateway resources.", zap.Error(err))
	}
}

func mustPodNamespace(ctx context.Context) string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		contextutils.LoggerFrom(ctx).Fatalw("POD_NAMESPACE is not set.")
	}
	return namespace
}
