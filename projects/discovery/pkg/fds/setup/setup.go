package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"

	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
)

func Main(customCtx context.Context) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggerName:  "fds",
		Version:     version.Version,
		SetupFunc:   syncer.NewSetupFunc(),
		ExitOnError: true,
		CustomCtx:   customCtx,
	})
}
