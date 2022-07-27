package runner

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Run(ctx context.Context) error {
	runnerOptions := bootstrap.RunnerOpts{
		Ctx:        ctx,
		LoggerName: "ingress",
		Version:    version.Version,
		SetupFunc:  Setup,
	}
	return bootstrap.Run(runnerOptions)
}
