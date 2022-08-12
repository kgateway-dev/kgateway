package setup

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"

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

		ElectionConfig: &leaderelector.ElectionConfig{
			Id:        "gloo",
			Namespace: utils.GetPodNamespace(),
			// no-op all the callbacks for now
			// at the moment, leadership functionality is performed within components
			// in the future we could pull that out and let these callbacks change configuration
			OnStartedLeading: func(c context.Context) {
				contextutils.LoggerFrom(c).Info("starting leadership")
			},
			OnNewLeader: func(leaderId string) {
				contextutils.LoggerFrom(ctx).Infof("new leader elected with ID: %s", leaderId)
			},
			OnStoppedLeading: func() {
				// In Gloo Edge, Settings changes cause the entire setup loop to be re-run. Therefore,
				// leadership may be lost as a result of re-running setup.
				// Instead of killing the app, we will log a notification
				contextutils.LoggerFrom(ctx).Fatalf("lost leadership, continuing app")
			},
		},
	})
}
