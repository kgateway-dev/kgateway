package setup

import (
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/projects/gateway/pkg/syncer"
)

func Main() error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggingPrefix: "gateway",
		SetupFunc:     syncer.Setup,
		ExitOnError:   true,
	})
}
