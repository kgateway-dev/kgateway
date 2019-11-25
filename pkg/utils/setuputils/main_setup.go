package setuputils

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/solo-io/reporting-client/pkg/signature"

	"github.com/solo-io/gloo/pkg/utils/usage"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/reporting-client/pkg/client"

	"go.uber.org/zap"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

type SetupOpts struct {
	LoggerName        string
	LoggingPrefixVals []interface{}
	SetupFunc         SetupFunc
	ExitOnError       bool
	CustomCtx         context.Context

	// optional- if present, report usage with the payload this discovers
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
	ctx = contextutils.WithLoggerValues(ctx, opts.LoggingPrefixVals...)


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

	if err := writeDefaultSettings(setupNamespace, setupName, settingsClient); err != nil {
		return err
	}

	emitter := v1.NewSetupEmitter(settingsClient)
	settingsRef := core.ResourceRef{Namespace: setupNamespace, Name: setupName}
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
		return v1.NewSettingsClient(&factory.FileResourceClientFactory{
			RootDir: settingsDir,
		})
	}
	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, err
	}
	return v1.NewSettingsClient(&factory.KubeResourceClientFactory{
		Crd:                v1.SettingsCrd,
		Cfg:                cfg,
		SharedCache:        kube.NewKubeCache(ctx),
		NamespaceWhitelist: []string{setupNamespace},
		SkipCrdCreation:    settingsutil.GetSkipCrdCreation(),
	})
}

// TODO(ilackarms): remove this or move it to a test package, only use settings watch for production gloo
func writeDefaultSettings(defaultNamespace, name string, cli v1.SettingsClient) error {
	settings := &v1.Settings{
		ConfigSource: &v1.Settings_KubernetesConfigSource{
			KubernetesConfigSource: &v1.Settings_KubernetesCrds{},
		},
		ArtifactSource: &v1.Settings_KubernetesArtifactSource{
			KubernetesArtifactSource: &v1.Settings_KubernetesConfigmaps{},
		},
		SecretSource: &v1.Settings_KubernetesSecretSource{
			KubernetesSecretSource: &v1.Settings_KubernetesSecrets{},
		},
		Gloo: &v1.GlooOptions{
			XdsBindAddr: fmt.Sprintf("0.0.0.0:%v", defaults.GlooXdsPort),
		},
		RefreshRate:        types.DurationProto(time.Minute),
		DevMode:            true,
		DiscoveryNamespace: defaultNamespace,
		Metadata:           core.Metadata{Namespace: defaultNamespace, Name: name},
	}
	if _, err := cli.Write(settings, clients.WriteOpts{}); err != nil && !errors.IsExist(err) {
		return errors.Wrapf(err, "failed to create default settings")
	}
	return nil
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
