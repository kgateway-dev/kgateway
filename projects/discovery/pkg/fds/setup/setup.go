package setup

import (
	"context"

	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
	gloosyncer "github.com/solo-io/gloo/projects/gloo/pkg/syncer"
)

func Main(customCtx context.Context) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggerName:  "fds",
		Version:     version.Version,
		SetupFunc:   gloosyncer.NewSetupFuncWithRun(syncer.RunFDS),
		ExitOnError: true,
		CustomCtx:   customCtx,
	})
}
