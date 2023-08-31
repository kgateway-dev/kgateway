package setup

import (
	"context"
	"os"

	"github.com/go-logr/zapr"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log"
	zaputil "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	glooComponentName = "gloo"
	logLevelEnv       = "LOG_LEVEL"
)

func Main(customCtx context.Context) error {
	setupLogging(customCtx)
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

func setupLogging(ctx context.Context) {
	// set up controller-runtime logging
	level := zapcore.InfoLevel
	// if log level is set in env, use that
	if envLogLevel := os.Getenv(logLevelEnv); envLogLevel != "" {
		if err := (&level).Set(envLogLevel); err != nil {
			contextutils.LoggerFrom(ctx).Infof("Could not set log level from env %s=%s, available levels "+
				"can be found here: https://pkg.go.dev/go.uber.org/zap/zapcore?tab=doc#Level",
				logLevelEnv,
				envLogLevel,
				zap.Error(err),
			)
		}
	}
	atomicLevel := zap.NewAtomicLevelAt(level)

	baseLogger := zaputil.NewRaw(
		zaputil.Level(&atomicLevel),
		zaputil.RawZapOpts(zap.Fields(zap.String("version", version.Version))),
	).Named(glooComponentName)

	// controller-runtime
	log.SetLogger(zapr.NewLogger(baseLogger))
}
