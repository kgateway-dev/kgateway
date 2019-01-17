package setup

import (
	"github.com/solo-io/gloo/pkg/utils/setuputils"
)

func Main() error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggingPrefix: "ingress",
		SetupFunc:     Setup,
		ExitOnError:   true,
	})
}
