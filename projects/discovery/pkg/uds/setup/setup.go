package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/discovery/pkg/uds/runner"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Main(ctx context.Context) error {
	setupOptions := bootstrap.SetupOpts{
		Ctx:        ctx,
		LoggerName: "uds",
		Version:    version.Version,
		Runner:     runner.NewUDSRunner(),
	}

	return bootstrap.Setup(setupOptions)
}
