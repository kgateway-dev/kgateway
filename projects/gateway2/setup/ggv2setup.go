package setup

import (
	"context"
	"fmt"
	"net"
	"os"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/solo-io/gloo/pkg/utils/envutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/namespaces"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/projects/gateway2/admin"
	"github.com/solo-io/gloo/projects/gateway2/controller"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/go-utils/contextutils"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var settingsGVR = glookubev1.SchemeGroupVersion.WithResource("settings")

func createKubeClient(restConfig *rest.Config) (istiokube.Client, error) {
	restCfg := istiokube.NewClientConfigForRestConfig(restConfig)
	client, err := istiokube.NewClient(restCfg, "")
	if err != nil {
		return nil, err
	}
	istiokube.EnableCrdWatcher(client)
	return client, nil
}

func getInitialSettings(ctx context.Context, c istiokube.Client, nns types.NamespacedName) *glookubev1.Settings {
	// get initial settings
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("getting initial settings. gvr: %v", settingsGVR)

	i, err := c.Dynamic().Resource(settingsGVR).Namespace(nns.Namespace).Get(ctx, nns.Name, metav1.GetOptions{})
	if err != nil {
		logger.Panicf("failed to get initial settings: %v", err)
		return nil
	}
	logger.Infof("got initial settings")

	var empty glookubev1.Settings
	out := &empty
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(i.UnstructuredContent(), out)
	if err != nil {
		logger.Panicf("failed converting unstructured into settings: %v", i)
		return nil
	}
	return out
}

func StartGGv2(ctx context.Context,
	extraPlugins []extensionsplug.Plugin,
	extraGwClasses []string, // TODO: we can remove this and replace with something that watches all GW classes with our controller name

) error {
	restConfig := ctrl.GetConfigOrDie()

	uniqueClientCallbacks, uccBuilder := krtcollections.NewUniquelyConnectedClients()
	cache, err := startControlPlane(ctx, uniqueClientCallbacks)
	if err != nil {
		return err
	}

	setupOpts := &controller.SetupOpts{
		Cache:               cache,
		KrtDebugger:         new(krt.DebugHandler),
		ExtraGatewayClasses: extraGwClasses,
		XdsHost:             GetControlPlaneXdsHost(),
		XdsPort:             9977,
	}

	return StartGGv2WithConfig(ctx, setupOpts, restConfig, uccBuilder, extraPlugins, nil, setuputils.SetupNamespaceName())
}

// GetControlPlaneXdsHost gets the xDS address from the gloo Service.
func GetControlPlaneXdsHost() string {
	return kubeutils.ServiceFQDN(metav1.ObjectMeta{
		Name:      kubeutils.GlooServiceName,
		Namespace: namespaces.GetPodNamespace(),
	})
}

func startControlPlane(ctx context.Context,
	callbacks xdsserver.Callbacks) (envoycache.SnapshotCache, error) {

	return NewControlPlane(ctx, &net.TCPAddr{IP: net.IPv4zero, Port: 9977}, callbacks)
}

func StartGGv2WithConfig(ctx context.Context, setupOpts *controller.SetupOpts,
	restConfig *rest.Config,
	uccBuilder krtcollections.UniquelyConnectedClientsBulider,
	extraPlugins []extensionsplug.Plugin,
	extraGwClasses []string, // TODO: we can remove this and replace with something that watches all GW classes with our controller name
	settingsNns types.NamespacedName,
) error {
	ctx = contextutils.WithLogger(ctx, "k8s")

	logger := contextutils.LoggerFrom(ctx)
	logger.Info("starting gloo gateway")

	kubeClient, err := createKubeClient(restConfig)
	if err != nil {
		return err
	}

	initialSettings := getInitialSettings(ctx, kubeClient, settingsNns)
	if initialSettings == nil {
		return fmt.Errorf("initial settings not found")
	}

	logger.Info("creating krt collections")
	krtOpts := krtutil.NewKrtOptions(ctx.Done(), setupOpts.KrtDebugger)

	augmentedPods := krtcollections.NewPodsCollection(kubeClient, krtOpts)
	augmentedPodsForUcc := augmentedPods
	if envutils.IsEnvTruthy("DISABLE_POD_LOCALITY_XDS") {
		augmentedPodsForUcc = nil
	}

	ucc := uccBuilder(ctx, krtOpts, augmentedPodsForUcc)

	setting := krtutil.SetupCollectionDynamic[glookubev1.Settings](
		ctx,
		kubeClient,
		settingsGVR,
		krt.WithName("GlooSettings"))
	settingsSingle := krt.NewSingleton(func(ctx krt.HandlerContext) *glookubev1.Settings {
		s := krt.FetchOne(ctx, setting,
			krt.FilterObjectName(settingsNns))
		if s != nil {
			return *s
		}
		return nil
	}, krt.WithName("GlooSettingsSingleton"))

	logger.Info("initializing controller")
	c, err := controller.NewControllerBuilder(ctx, controller.StartConfig{
		ExtraPlugins:  extraPlugins,
		RestConfig:    restConfig,
		SetupOpts:     setupOpts,
		Client:        kubeClient,
		AugmentedPods: augmentedPods,
		UniqueClients: ucc,

		InitialSettings: initialSettings,
		Settings:        settingsSingle,
		// Dev flag may be useful for development purposes; not currently tied to any user-facing API
		Dev:        os.Getenv("LOG_LEVEL") == "debug",
		KrtOptions: krtOpts,
	})
	if err != nil {
		logger.Error("failed initializing controller: ", err)
		return err
	}
	/// no collections after this point

	logger.Info("waiting for cache sync")
	kubeClient.RunAndWait(ctx.Done())
	setting.Synced().WaitUntilSynced(ctx.Done())

	logger.Info("starting admin server")
	go admin.RunAdminServer(ctx, setupOpts)

	logger.Info("starting controller")
	return c.Start(ctx)
}
