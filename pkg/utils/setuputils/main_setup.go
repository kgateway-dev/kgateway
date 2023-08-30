package setuputils

import (
	"context"
	"flag"
	"os"
	"sync"
	"time"

	"github.com/go-logr/zapr"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	kube2 "github.com/solo-io/gloo/pkg/bootstrap/leaderelector/kube"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector/singlereplica"
	"github.com/solo-io/gloo/pkg/version"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/log"
	zaputil "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type SetupOpts struct {
	LoggerName string
	// logged as the version of Gloo currently executing
	Version     string
	SetupFunc   SetupFunc
	ExitOnError bool
	CustomCtx   context.Context

	// optional - if present, add these values in each JSON log line in the gloo pod.
	// By default, we already log the gloo version.
	LoggingPrefixVals []interface{}
	// optional - if present, report usage with the payload this discovers
	// should really only provide it in very intentional places- in the gloo pod, and in glooctl
	// otherwise, we'll provide redundant copies of the usage data

	ElectionConfig *leaderelector.ElectionConfig
}

var once sync.Once

const (
	LOG_LEVEL = "LOG_LEVEL"
)

// Main is the main entrypoint for running Gloo Edge components
// It works by performing the following:
//  1. Initialize a SettingsClient backed either by Kubernetes or a File
//  2. Run an event loop, watching events on the Settings resource, and executing the
//     opts.SetupFunc whenever settings change
//
// This allows Gloo components to automatically receive updates to Settings and reload their
// configuration, without needing to restart the container
func Main(opts SetupOpts) error {
	// prevent panic if multiple flag.Parse called concurrently
	once.Do(func() {
		flag.Parse()
	})

	ctx := opts.CustomCtx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = contextutils.WithLogger(ctx, opts.LoggerName)
	loggingContext := append([]interface{}{"version", opts.Version}, opts.LoggingPrefixVals...)
	ctx = contextutils.WithLoggerValues(ctx, loggingContext...)

	// set up controller-runtime logging
	level := zapcore.InfoLevel
	// if log level is set in env, use that
	if envLogLevel := os.Getenv(LOG_LEVEL); envLogLevel != "" {
		if err := (&level).Set(envLogLevel); err != nil {
			contextutils.LoggerFrom(ctx).Infof("Could not set log level from env %s=%s, available levels "+
				"can be found here: https://pkg.go.dev/go.uber.org/zap/zapcore?tab=doc#Level",
				LOG_LEVEL,
				envLogLevel,
				zap.Error(err),
			)
		}
	}
	atomicLevel := zap.NewAtomicLevelAt(level)

	baseLogger := zaputil.NewRaw(
		zaputil.Level(&atomicLevel),
		zaputil.RawZapOpts(zap.Fields(zap.String("version", version.Version))),
	).Named(opts.LoggerName)

	// klog
	zap.ReplaceGlobals(baseLogger)
	// controller-runtime
	log.SetLogger(zapr.NewLogger(baseLogger))
	// go-utils
	contextutils.SetFallbackLogger(baseLogger.Sugar())

	settingsClient, err := fileOrKubeSettingsClient(ctx, setupNamespace, setupDir)
	if err != nil {
		return err
	}

	if err := settingsClient.Register(); err != nil {
		return err
	}

	identity, err := startLeaderElection(ctx, setupDir, opts.ElectionConfig)
	if err != nil {
		return err
	}

	// settings come from the ResourceClient in the settingsClient
	// the eventLoop will Watch the emitter's settingsClient to recieve settings from the ResourceClient
	emitter := v1.NewSetupEmitter(settingsClient)
	settingsRef := &core.ResourceRef{Namespace: setupNamespace, Name: setupName}
	eventLoop := v1.NewSetupEventLoop(emitter, NewSetupSyncer(settingsRef, opts.SetupFunc, identity))
	errs, err := eventLoop.Run([]string{setupNamespace}, clients.WatchOpts{
		Ctx:         ctx,
		RefreshRate: time.Second,
	})
	if err != nil {
		return err
	}
	for err := range errs {
		if opts.ExitOnError {
			contextutils.LoggerFrom(ctx).Fatalf("error in setup: %v", err)
		}
		contextutils.LoggerFrom(ctx).Errorf("error in setup: %v", err)
	}
	return nil
}

func fileOrKubeSettingsClient(ctx context.Context, setupNamespace, settingsDir string) (v1.SettingsClient, error) {
	if settingsDir != "" {
		contextutils.LoggerFrom(ctx).Infow("using filesystem for settings", zap.String("directory", settingsDir))
		return v1.NewSettingsClient(ctx, &factory.FileResourceClientFactory{
			RootDir: settingsDir,
		})
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, err
	}
	return v1.NewSettingsClient(ctx, &factory.KubeResourceClientFactory{
		Crd:                v1.SettingsCrd,
		Cfg:                cfg,
		SharedCache:        kube.NewKubeCache(ctx),
		NamespaceWhitelist: []string{setupNamespace},
	})
}

func startLeaderElection(ctx context.Context, settingsDir string, electionConfig *leaderelector.ElectionConfig) (leaderelector.Identity, error) {
	if electionConfig == nil || settingsDir != "" || leaderelector.IsDisabled() {
		// If a component does not contain election config, it does not support HA
		// If the settingsDir is non-empty, it means that Settings are not defined in Kubernetes and therefore we can't use the
		// leader election library which depends on Kubernetes
		// If leader election is explicitly disabled, it means a user has decided not to opt-into HA
		return singlereplica.NewElectionFactory().StartElection(ctx, electionConfig)
	}

	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, err
	}
	return kube2.NewElectionFactory(cfg).StartElection(ctx, electionConfig)
}
