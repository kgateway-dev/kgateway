package setup

import (
	"context"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/discovery/pkg/uds/syncer"
	gloosyncer "github.com/solo-io/gloo/projects/gloo/pkg/syncer"
)

func Main(customCtx context.Context) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggerName:        "uds",
		LoggingPrefixVals: []interface{}{"version", version.Version},
		SetupFunc:         gloosyncer.NewSetupFuncWithRun(syncer.RunUDS),
		ExitOnError:       true,
		CustomCtx:         customCtx,
	})
}
