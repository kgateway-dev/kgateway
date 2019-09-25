package main

import (
	"context"
	"os"

	"github.com/solo-io/gloo/projects/gloo/pkg/setup"
	"github.com/solo-io/gloo/projects/metrics/pkg/runner"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/stats"
	"go.uber.org/zap"
)

const (
	START_STATS_SERVER = "START_STATS_SERVER"
)

func main() {
	if os.Getenv(START_STATS_SERVER) != "" {
		stats.StartStatsServer()
	}
	podNamespace := os.Getenv("POD_NAMESPACE")
	go func() {
		ctx := context.Background()
		if err := runner.RunE(ctx, podNamespace); err != nil {
			contextutils.LoggerFrom(ctx).Errorw("err in metrics server", zap.Error(err))
		}
	}()
	if err := setup.Main(nil); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}
