package setuputils

import (
	"context"
	"flag"
	"sync"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/version"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	check "github.com/solo-io/go-checkpoint"
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
	LoggingPrefix string
	SetupFunc     SetupFunc
	ExitOnError   bool
	CustomCtx     context.Context
}

var once sync.Once

func Main(opts SetupOpts) error {
	start := time.Now()
	loggingPrefix := opts.LoggingPrefix
	check.CallCheck(loggingPrefix, version.Version, start)
	// prevent panic if multiple flag.Parse called concurrently
	once.Do(func() {
		flag.Parse()
	})

	ctx := opts.CustomCtx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = contextutils.WithLogger(ctx, loggingPrefix)

	settingsClient, err := kubeOrFileSettingsClient(ctx, setupDir)
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

func kubeOrFileSettingsClient(ctx context.Context, settingsDir string) (v1.SettingsClient, error) {
	if settingsDir != "" {
		return v1.NewSettingsClient(&factory.FileResourceClientFactory{
			RootDir: settingsDir,
		})
	}
	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, err
	}
	return v1.NewSettingsClient(&factory.KubeResourceClientFactory{
		Crd:         v1.SettingsCrd,
		Cfg:         cfg,
		SharedCache: kube.NewKubeCache(ctx),
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
		BindAddr:           "0.0.0.0:9977",
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
