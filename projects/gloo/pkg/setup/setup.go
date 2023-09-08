package setup

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/go-utils/contextutils"
)

const (
	glooComponentName = "gloo"
)

func Main(customCtx context.Context) error {
	setuputils.SetupLogging(customCtx, glooComponentName)
	return startSetupLoop(customCtx)
}

func startSetupLoop(ctx context.Context) error {
	return setuputils.Main(setuputils.SetupOpts{
		LoggerName:  glooComponentName,
		Version:     version.Version,
		SetupFunc:   setup.NewSetupFunc(),
		ExitOnError: true,
		CustomCtx:   ctx,

		ElectionConfig: &leaderelector.ElectionConfig{
			Id:        glooComponentName,
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
				// Kill app if we lose leadership, we need to be VERY sure we don't continue
				// any leader election processes.
				// https://github.com/solo-io/gloo/issues/7346
				// There is follow-up work to handle lost leadership more gracefully
				contextutils.LoggerFrom(ctx).Fatalf("lost leadership, quitting app")
			},
		},
	})
}
