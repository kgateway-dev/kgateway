package runner

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/discovery/pkg/uds/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
)

func Run(customCtx context.Context) error {
	runnerOptions := bootstrap.RunnerOpts{
		Ctx:        customCtx,
		LoggerName: "uds",
		Version:    version.Version,
		SetupFunc:  setup.NewSetupFuncWithRun(syncer.RunUDS),
	}

	return bootstrap.Run(runnerOptions)
}
