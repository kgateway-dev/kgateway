package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/projects/discovery/pkg/uds/syncer"
)

func Main(customCtx context.Context) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggerName:  "uds",
		Version:     version.Version,
		SetupFunc:   setup.NewSetupFuncWithRun(syncer.RunUDS),
		ExitOnError: true,
		CustomCtx:   customCtx,
	})
}
