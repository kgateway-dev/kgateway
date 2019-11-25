package setup

import (
	"context"
	"github.com/solo-io/gloo/pkg/version"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
)

func Main(customCtx context.Context) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggerName:  "ingress",
		LoggingPrefixVals: []interface{}{"version", version.Version},
		SetupFunc:   Setup,
		ExitOnError: true,
		CustomCtx:   customCtx,
	})
}
