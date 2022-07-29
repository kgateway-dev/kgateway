package runner

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Run(ctx context.Context) error {
	runnerOptions := bootstrap.SetupOpts{
		Ctx:           ctx,
		LoggerName:    "ingress",
		Version:       version.Version,
		RunnerFactory: Setup,
	}
	return bootstrap.Setup(runnerOptions)
}
