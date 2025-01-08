package setup

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
	ggv2setup "github.com/solo-io/gloo/projects/gateway2/setup"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
)

const (
	glooComponentName = "gloo"
)

func Main(customCtx context.Context) error {
	setuputils.SetupLogging(customCtx, glooComponentName)
	return startSetupLoop(customCtx)
}

func startSetupLoop(ctx context.Context) error {
	return ggv2setup.StartGGv2(ctx, nil, nil)
}

func newSetupFunc(setupOpts *bootstrap.SetupOpts) setuputils.SetupFunc {

	runFunc := func(opts bootstrap.Opts) error {
		return setup.RunGloo(opts)
	}

	return setup.NewSetupFuncWithRunAndExtensions(runFunc, setupOpts, nil)
}
