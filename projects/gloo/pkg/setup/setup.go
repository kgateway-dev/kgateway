package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
)

func Main(customCtx context.Context) error {
	return startSetupLoop(customCtx)
}

func StartGlooInTest(customCtx context.Context) error {
	return startSetupLoop(customCtx)
}

func startSetupLoop(ctx context.Context) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggerName:  "gloo",
		Version:     version.Version,
		SetupFunc:   setup.NewSetupFunc(),
		ExitOnError: true,
		CustomCtx:   ctx,
	})
}
