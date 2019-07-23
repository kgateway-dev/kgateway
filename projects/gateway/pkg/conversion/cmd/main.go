package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/solo-io/gloo/projects/gateway/pkg/conversion"
	"github.com/solo-io/gloo/projects/gateway/pkg/conversion/setup"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
)

func main() {
	ctx := contextutils.WithLogger(context.Background(), "gateway-conversion")
	clientSet := setup.MustClientSet(ctx)

	var resourceConverter conversion.ResourceConverter = conversion.NewResourceConverter(
		mustPodNamespace(ctx),
		clientSet.V1Gateway,
		clientSet.V2Gateway,
		conversion.NewGatewayConverter(),
	)

	sigs := make(chan os.Signal, 1)
	exit := make(chan int, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		timeToExit := 10 * time.Second
		contextutils.LoggerFrom(ctx).Infof("Received %v. Will exit in %v", sig.String(), timeToExit.String())
		time.Sleep(timeToExit)
		exit <- 1
	}()

	go func() {
		attemptCount := 5
		for i := 0; i < attemptCount; i++ {
			if err := resourceConverter.ConvertAll(ctx); err != nil {
				contextutils.LoggerFrom(ctx).Errorw("Error encountered while upgrading gateway resources", zap.Error(err))
			} else {
				exit <- 0
				return
			}
		}
		contextutils.LoggerFrom(ctx).Errorw("Failed to convert all v1 resources after %v attempts", attemptCount)
		exit <- 1
	}()

	os.Exit(<-exit)
}

func mustPodNamespace(ctx context.Context) string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		contextutils.LoggerFrom(ctx).Fatal("POD_NAMESPACE is not set.")
	}
	return namespace
}
