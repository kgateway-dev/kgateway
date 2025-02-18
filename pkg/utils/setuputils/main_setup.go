package setuputils

import (
	"context"
	"os"

	"github.com/go-logr/zapr"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/log"
	zaputil "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kgateway-dev/kgateway/v2/internal/version"
)

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
