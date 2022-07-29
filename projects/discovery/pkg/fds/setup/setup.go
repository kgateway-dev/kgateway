package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds/runner"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Main(customCtx context.Context) error {
	setupOptions := bootstrap.SetupOpts{
		Ctx:           customCtx,
		LoggerName:    "fds",
		Version:       version.Version,
		RunnerFactory: runner.NewRunnerFactory(),
	}
	return bootstrap.Setup(setupOptions)
}
