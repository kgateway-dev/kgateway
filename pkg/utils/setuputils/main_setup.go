package setuputils

import (
	"context"
	"os"

	"github.com/kgateway-dev/kgateway/pkg/utils/kubeutils"

	"github.com/go-logr/zapr"
	"github.com/kgateway-dev/kgateway/pkg/bootstrap/leaderelector"
	kube2 "github.com/kgateway-dev/kgateway/pkg/bootstrap/leaderelector/kube"
	"github.com/kgateway-dev/kgateway/pkg/bootstrap/leaderelector/singlereplica"
	"github.com/kgateway-dev/kgateway/pkg/version"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/log"
	zaputil "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func startLeaderElection(ctx context.Context, settingsDir string, electionConfig *leaderelector.ElectionConfig) (leaderelector.Identity, error) {
	if electionConfig == nil || settingsDir != "" || leaderelector.IsDisabled() {
		// If a component does not contain election config, it does not support HA
		// If the settingsDir is non-empty, it means that Settings are not defined in Kubernetes and therefore we can't use the
		// leader election library which depends on Kubernetes
		// If leader election is explicitly disabled, it means a user has decided not to opt-into HA
		return singlereplica.NewElectionFactory().StartElection(ctx, electionConfig)
	}

	cfg, err := kubeutils.GetRestConfigWithKubeContext("")
	if err != nil {
		return nil, err
	}
	return kube2.NewElectionFactory(cfg).StartElection(ctx, electionConfig)
}

// SetupLogging sets up controller-runtime logging
func SetupLogging(ctx context.Context, loggerName string) {
	level := zapcore.InfoLevel
	// if log level is set in env, use that
	if envLogLevel := os.Getenv(contextutils.LogLevelEnvName); envLogLevel != "" {
		if err := (&level).Set(envLogLevel); err != nil {
			contextutils.LoggerFrom(ctx).Infof("Could not set log level from env %s=%s, available levels "+
				"can be found here: https://pkg.go.dev/go.uber.org/zap/zapcore?tab=doc#Level",
				contextutils.LogLevelEnvName,
				envLogLevel,
				zap.Error(err),
			)
		}
	}
	atomicLevel := zap.NewAtomicLevelAt(level)

	baseLogger := zaputil.NewRaw(
		zaputil.Level(&atomicLevel),
		zaputil.RawZapOpts(zap.Fields(zap.String("version", version.Version))),
	).Named(loggerName)

	// controller-runtime
	log.SetLogger(zapr.NewLogger(baseLogger))
}
