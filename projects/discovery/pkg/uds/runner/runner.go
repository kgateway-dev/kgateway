package runner

import (
	"context"
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Run(customCtx context.Context) error {
	runnerOptions := bootstrap.RunnerOpts{
		Ctx:        customCtx,
		LoggerName: "uds",
		Version:    version.Version,
		SetupFunc:  runner.NewSetupFuncWithRun(StartUDS),
	}

	return bootstrap.Run(runnerOptions)
}
