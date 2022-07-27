package runner

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/gloo/pkg/version"
)

func Run(ctx context.Context) error {
	runnerOptions := bootstrap.RunnerOpts{
		Ctx:        ctx,
		LoggerName: "gloo",
		Version:    version.Version,
		SetupFunc:  setup.NewSetupFunc(),
	}
	return bootstrap.Run(runnerOptions)
}
