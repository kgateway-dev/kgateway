package runner

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Run(customCtx context.Context) error {
	runnerOptions := bootstrap.RunnerOpts{
		Ctx:        customCtx,
		LoggerName: "fds",
		Version:    version.Version,
		SetupFunc:  NewSetupFunc(),
	}
	return bootstrap.Run(runnerOptions)
}
