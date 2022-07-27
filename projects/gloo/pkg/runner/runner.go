package runner

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Run(ctx context.Context) error {
	runnerOptions := bootstrap.RunnerOpts{
		Ctx:        ctx,
		LoggerName: "gloo",
		Version:    version.Version,
		SetupFunc:  NewSetupFunc(),
	}
	return bootstrap.Run(runnerOptions)
}
