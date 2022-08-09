package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds/runner"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Main(ctx context.Context) error {
	setupOptions := bootstrap.SetupOpts{
		Ctx:        ctx,
		LoggerName: "fds",
		Version:    version.Version,
		Runner:     runner.NewFDSRunner(),
	}
	return bootstrap.Setup(setupOptions)
}
