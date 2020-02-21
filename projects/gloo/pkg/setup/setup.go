package setup

import (
	"context"
	"os"

	"github.com/solo-io/gloo/pkg/version"

	"go.uber.org/zap"

	"github.com/solo-io/gloo/pkg/utils/usage"
	"github.com/solo-io/gloo/projects/metrics/pkg/metricsservice"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/reporting-client/pkg/client"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
)

func Main(customCtx context.Context) error {
	var usageReporter client.UsagePayloadReader
	metricsStorage, err := metricsservice.NewDefaultConfigMapStorage(os.Getenv("POD_NAMESPACE"))
	if err != nil {
		contextutils.LoggerFrom(customCtx).Warnw("Could not create metrics storage loader - will not report usage: %s", zap.Error(err))
	} else {
		usageReporter = &usage.DefaultUsageReader{MetricsStorage: metricsStorage}
	}

	return startSetupLoop(customCtx, usageReporter)
}

func StartGlooInTest(customCtx context.Context) error {
	return startSetupLoop(customCtx, nil)
}

func startSetupLoop(ctx context.Context, usageReporter client.UsagePayloadReader) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggerName:    "gloo",
		Version:       version.Version,
		SetupFunc:     syncer.NewSetupFunc(),
		ExitOnError:   true,
		CustomCtx:     ctx,
		UsageReporter: usageReporter,
	})
}
