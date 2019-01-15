package setuputils

import (
	"context"
	"flag"
	"time"

	"github.com/gogo/protobuf/types"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
)

func Main(loggingPrefix string, setupFunc SetupFunc) error {
	flag.Parse()

	settingsClient, err := KubeOrFileSettingsClient(setupDir)
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
	ctx := contextutils.WithLogger(context.Background(), loggingPrefix)
	settingsRef := core.ResourceRef{Namespace: setupNamespace, Name: setupName}
	eventLoop := v1.NewSetupEventLoop(emitter, NewSetupSyncer(settingsRef, setupFunc))
	errs, err := eventLoop.Run([]string{setupNamespace}, clients.WatchOpts{
		Ctx:         ctx,
		RefreshRate: time.Second,
	})
	if err != nil {
		return err
	}
	for err := range errs {
		contextutils.LoggerFrom(ctx).Errorf("error in setup: %v", err)
	}
	return nil
}

// TODO (ilackarms): instead of using an heuristic here, read from a CLI flagg
// first attempt to use kube crd, otherwise fall back to file
func KubeOrFileSettingsClient(settingsDir string) (v1.SettingsClient, error) {
	cfg, err := kubeutils.GetConfig("", "")
	if err == nil {
		return v1.NewSettingsClient(&factory.KubeResourceClientFactory{
			Crd:         v1.SettingsCrd,
			Cfg:         cfg,
			SharedCache: kube.NewKubeCache(),
		})
	}
	return v1.NewSettingsClient(&factory.FileResourceClientFactory{
		RootDir: settingsDir,
	})
}

// TODO(ilackarms): remove this or move it to a test package, only use settings watch for production gloo
func writeDefaultSettings(settingsNamespace, name string, cli v1.SettingsClient) error {
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
		DiscoveryNamespace: settingsNamespace,
		Metadata:           core.Metadata{Namespace: settingsNamespace, Name: name},
	}
	if _, err := cli.Write(settings, clients.WriteOpts{}); err != nil && !errors.IsExist(err) {
		return errors.Wrapf(err, "failed to create default settings")
	}
	return nil
}
