package setup

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
)

func Main(ctx context.Context) error {
	setupOptions := bootstrap.SetupOpts{
		Ctx:        ctx,
		LoggerName: "gloo",
		Version:    version.Version,
		Runner:     runner.NewGlooRunner(),
	}
	return bootstrap.Setup(setupOptions)
}
