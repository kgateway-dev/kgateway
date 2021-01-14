package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/pkg/utils/usage"
	"github.com/solo-io/reporting-client/pkg/client"
)

func Main(customCtx context.Context) error {
	usageReporter := &usage.DefaultUsageReader{}
	return startSetupLoop(customCtx, usageReporter)
}

func StartGlooInTest(customCtx context.Context) error {
	return startSetupLoop(customCtx, nil)
}

func startSetupLoop(ctx context.Context, usageReporter client.UsagePayloadReader) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggerName:    "gloo",
		Version:       version.Version,
		SetupFunc:     setup.NewSetupFunc(),
		ExitOnError:   true,
		CustomCtx:     ctx,
		UsageReporter: usageReporter,
	})
}
