package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/ingress/pkg/runner"

	"github.com/solo-io/gloo/pkg/bootstrap"

	"github.com/solo-io/gloo/pkg/version"
)

func Run(ctx context.Context) error {
	setupOptions := bootstrap.SetupOpts{
		Ctx:        ctx,
		LoggerName: "ingress",
		Version:    version.Version,
		Runner:     runner.NewIngressRunner(),
	}
	return bootstrap.Setup(setupOptions)
}
