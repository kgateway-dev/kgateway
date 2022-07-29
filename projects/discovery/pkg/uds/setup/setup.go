package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/runner"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Main(customCtx context.Context) error {
	setupOptions := bootstrap.SetupOpts{
		Ctx:           customCtx,
		LoggerName:    "uds",
		Version:       version.Version,
		RunnerFactory: runner.NewRunnerFactory().GetRunnerFactory(),
	}

	return bootstrap.Setup(setupOptions)
}
