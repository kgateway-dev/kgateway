package setup

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
)

func Main(customCtx context.Context) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggingPrefix: "ingress",
		SetupFunc:     Setup,
		ExitOnError:   true,
		CustomCtx:     customCtx,
	})
}
