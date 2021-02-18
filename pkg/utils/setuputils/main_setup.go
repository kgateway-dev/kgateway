package setuputils

import (
	"context"
	"flag"
	"sync"
	"time"

	"github.com/solo-io/gloo/pkg/utils/usage"
	"github.com/solo-io/gloo/pkg/version"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/reporting-client/pkg/client"
	"github.com/solo-io/reporting-client/pkg/signature"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
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
	UsageReporter client.UsagePayloadReader
}

var once sync.Once

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

	if opts.UsageReporter != nil {
		go func() {
			signatureManager := signature.NewSignatureManager()
			errs := StartReportingUsage(opts.CustomCtx, opts.UsageReporter, opts.LoggerName, signatureManager)
			for err := range errs {
				contextutils.LoggerFrom(ctx).Warnw("Error while reporting usage", zap.Error(err))
			}
		}()
	}

	settingsClient, err := kubeOrFileSettingsClient(ctx, setupNamespace, setupDir)
	if err != nil {
		return err
	}
	if err := settingsClient.Register(); err != nil {
		return err
	}

	emitter := v1.NewSetupEmitter(settingsClient)
	settingsRef := &core.ResourceRef{Namespace: setupNamespace, Name: setupName}
	eventLoop := v1.NewSetupEventLoop(emitter, NewSetupSyncer(settingsRef, opts.SetupFunc))
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

func kubeOrFileSettingsClient(ctx context.Context, setupNamespace, settingsDir string) (v1.SettingsClient, error) {
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

// does not block the current goroutine
func StartReportingUsage(ctx context.Context, usagePayloadReader client.UsagePayloadReader, product string, signatureManager signature.SignatureManager) <-chan error {
	usageClient := client.NewUsageClient(
		usage.ReportingServiceUrl,
		usagePayloadReader,
		usage.BuildProductMetadata(product, version.Version),
		signatureManager)

	return usageClient.StartReportingUsage(ctx, usage.ReportingPeriod)
}
