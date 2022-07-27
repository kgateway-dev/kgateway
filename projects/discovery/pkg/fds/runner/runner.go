package runner

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
)

func Run(customCtx context.Context) error {
	runnerOptions := bootstrap.RunnerOpts{
		Ctx:        customCtx,
		LoggerName: "fds",
		Version:    version.Version,
		SetupFunc:  syncer.NewSetupFunc(),
	}
	return bootstrap.Run(runnerOptions)
}
