package bootstrap

import (
	"context"
	"flag"
	"sync"
	"time"

	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

type SetupOpts struct {
	Ctx context.Context

	// LoggerName is the name of the logger, which corresponds to the component that is being run
	LoggerName string

	// Version is the current version of Gloo that is executing.
	Version string

	// RunnerFactory defines the behavior that will execute whenever Settings are updated
	RunnerFactory RunnerFactory
}

var once sync.Once

// Setup is the main entrypoint for running Gloo Edge components
// It works by performing the following:
//	1. Initialize a SettingsClient backed either by Kubernetes or a File
// 	2. Run an event loop, watching events on the Settings resource, and executing the
//		opts.RunnerFactory whenever settings change
// This allows Gloo components to automatically receive updates to Settings and reload their
// configuration, without needing to restart the container
func Setup(opts SetupOpts) error {
	// validate opts just to be safe
	if err := validateSetupOpts(opts); err != nil {
		return err
	}

	// prevent panic if multiple flag.Parse called concurrently
	once.Do(flag.Parse)

	// initialize the context with logging
	ctx := contextutils.WithLogger(opts.Ctx, opts.LoggerName)
	ctx = contextutils.WithLoggerValues(ctx, []interface{}{
		"version", opts.Version,
	})

	// instantiate the settings client
	settingsFactory, err := getSettingsResourceClientFactory(ctx, setupNamespace, setupDir)
	settingsClient, err := v1.NewSettingsClient(ctx, settingsFactory)
	if err != nil {
		return err
	}
	if err := settingsClient.Register(); err != nil {
		return err
	}

	// define the setup behavior which will occur when settings change
	settingsRef := &core.ResourceRef{Namespace: setupNamespace, Name: setupName}
	setupSyncer := NewSetupSyncer(settingsRef, opts.RunnerFactory)

	// run an event loop, watching events on the Settings resource
	emitter := v1.NewSetupEmitter(settingsClient)
	eventLoop := v1.NewSetupEventLoop(emitter, setupSyncer)
	eventLoopErrs, err := eventLoop.Run([]string{setupNamespace}, clients.WatchOpts{
		Ctx:         ctx,
		RefreshRate: time.Second,
	})
	if err != nil {
		return err
	}

	for eventLoopErr := range eventLoopErrs {
		contextutils.LoggerFrom(ctx).Fatalf("error in setup: %v", eventLoopErr)
	}
	return nil
}

// validateSetupOpts returns an error if any of the required SetupOpts are missing, nil otherwise
func validateSetupOpts(setupOpts SetupOpts) error {
	if setupOpts.Ctx == nil {
		return eris.New("Ctx required, found nil")
	}
	if setupOpts.RunnerFactory == nil {
		return eris.New("RunnerFactory required, found nil")
	}
	if setupOpts.LoggerName == "" {
		return eris.New("LoggerName required, found nil")
	}
	if setupOpts.Version == "" {
		return eris.New("Version required, found nil")
	}
	return nil
}

// getSettingsResourceClientFactory returns the factory.ResourceClientFactory used to power the Settings client
func getSettingsResourceClientFactory(ctx context.Context, setupNamespace, settingsDir string) (factory.ResourceClientFactory, error) {
	if settingsDir != "" {
		contextutils.LoggerFrom(ctx).Infow("using filesystem for settings", zap.String("directory", settingsDir))
		return &factory.FileResourceClientFactory{
			RootDir: settingsDir,
		}, nil
	}
	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, err
	}
	return &factory.KubeResourceClientFactory{
		Crd:                v1.SettingsCrd,
		Cfg:                cfg,
		SharedCache:        kube.NewKubeCache(ctx),
		NamespaceWhitelist: []string{setupNamespace},
	}, nil
}
