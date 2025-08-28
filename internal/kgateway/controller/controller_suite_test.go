package controller_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gleak"
	"github.com/onsi/gomega/types"
	"istio.io/istio/pkg/kube"
	istiosets "istio.io/istio/pkg/util/sets"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/setup"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/assertions"
)

const (
	gatewayClassName            = "clsname"
	altGatewayClassName         = "clsname-alt"
	selfManagedGatewayClassName = "clsname-selfmanaged"
	gatewayControllerName       = "kgateway.dev/kgateway"
	defaultNamespace            = "default"
)

var (
	cfg          *rest.Config
	k8sClient    client.Client
	testEnv      *envtest.Environment
	ctx          context.Context
	cancel       context.CancelFunc
	kubeconfig   string
	gwClasses    = sets.New(gatewayClassName, altGatewayClassName, selfManagedGatewayClassName)
	scheme       *runtime.Scheme
	inferenceExt *deployer.InferenceExtInfo
)

func getAssetsDir() string {
	var assets string
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// set default if not user provided
		out, err := exec.Command("sh", "-c", "make -sC $(dirname $(go env GOMOD)) envtest-path").CombinedOutput()
		fmt.Fprintln(GinkgoWriter, "out:", string(out))
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		assets = strings.TrimSpace(string(out))
	}
	return assets
}

var _ = BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	// Create a scheme and add both Gateway and InferencePool types.
	scheme = schemes.GatewayScheme()
	err := inf.Install(scheme)
	Expect(err).NotTo(HaveOccurred())
	// Required to deploy endpoint picker RBAC resources.
	err = rbacv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "crds"),
			filepath.Join("..", "..", "..", "install", "helm", "kgateway-crds", "templates"),
		},
		ErrorIfCRDPathMissing: true,
		// set assets dir so we can run without the makefile
		BinaryAssetsDirectory: getAssetsDir(),
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
	if kubeconfig != "" {
		os.Remove(kubeconfig)
	}
})

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

func generateKubeConfiguration(restconfig *rest.Config) string {
	clusters := make(map[string]*clientcmdapi.Cluster)
	authinfos := make(map[string]*clientcmdapi.AuthInfo)
	contexts := make(map[string]*clientcmdapi.Context)

	clusterName := "cluster"
	clusters[clusterName] = &clientcmdapi.Cluster{
		Server:                   restconfig.Host,
		CertificateAuthorityData: restconfig.CAData,
	}
	authinfos[clusterName] = &clientcmdapi.AuthInfo{
		ClientKeyData:         restconfig.KeyData,
		ClientCertificateData: restconfig.CertData,
	}
	contexts[clusterName] = &clientcmdapi.Context{
		Cluster:   clusterName,
		Namespace: "default",
		AuthInfo:  clusterName,
	}

	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "cluster",
		AuthInfos:      authinfos,
	}
	// create temp file
	tmpfile, err := os.CreateTemp("", "ggii_envtest_*.kubeconfig")
	Expect(err).NotTo(HaveOccurred())
	tmpfile.Close()
	err = clientcmd.WriteToFile(clientConfig, tmpfile.Name())
	Expect(err).NotTo(HaveOccurred())
	return tmpfile.Name()
}

type fakeDiscoveryNamespaceFilter struct{}

func (f fakeDiscoveryNamespaceFilter) Filter(obj any) bool {
	// this is a fake filter, so we just return true
	return true
}

func (f fakeDiscoveryNamespaceFilter) AddHandler(func(selected, deselected istiosets.String)) {
}

func createManager(
	parentCtx context.Context,
	inferenceExt *deployer.InferenceExtInfo,
	classConfigs map[string]*controller.ClassInfo,
) (context.CancelFunc, error) {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    testEnv.WebhookInstallOptions.LocalServingHost,
			Port:    testEnv.WebhookInstallOptions.LocalServingPort,
			CertDir: testEnv.WebhookInstallOptions.LocalServingCertDir,
		}),
		Controller: config.Controller{
			// see https://github.com/kubernetes-sigs/controller-runtime/issues/2937
			// in short, our tests reuse the same name (reasonably so) and the controller-runtime
			// package does not reset the stack of controller names between tests, so we disable
			// the name validation here.
			SkipNameValidation: ptr.To(true),
		},
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(parentCtx)
	kubeClient, _ := setup.CreateKubeClient(cfg)
	gwCfg := controller.GatewayConfig{
		Mgr:            mgr,
		ControllerName: gatewayControllerName,
		AutoProvision:  true,
		ImageInfo: &deployer.ImageInfo{
			Registry: "ghcr.io/kgateway-dev",
			Tag:      "latest",
		},
		DiscoveryNamespaceFilter: fakeDiscoveryNamespaceFilter{},
		CommonCollections:        newCommonCols(ctx, kubeClient),
	}
	if err := controller.NewBaseGatewayController(parentCtx, gwCfg, nil); err != nil {
		cancel()
		return nil, err
	}
	if err := mgr.GetClient().Create(ctx, &v1alpha1.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      selfManagedGatewayClassName,
			Namespace: "default",
		},
		Spec: v1alpha1.GatewayParametersSpec{
			SelfManaged: &v1alpha1.SelfManagedGateway{},
		},
	}); client.IgnoreAlreadyExists(err) != nil {
		cancel()
		return nil, err
	}

	// Use the default & alt GCs when no class configs are provided.
	if classConfigs == nil {
		classConfigs = map[string]*controller.ClassInfo{}
		classConfigs[altGatewayClassName] = &controller.ClassInfo{
			Description: "alt gateway class",
		}
		classConfigs[gatewayClassName] = &controller.ClassInfo{
			Description: "default gateway class",
		}
		classConfigs[selfManagedGatewayClassName] = &controller.ClassInfo{
			Description: "self managed gw",
			ParametersRef: &apiv1.ParametersReference{
				Group:     apiv1.Group(wellknown.GatewayParametersGVK.Group),
				Kind:      apiv1.Kind(wellknown.GatewayParametersGVK.Kind),
				Name:      selfManagedGatewayClassName,
				Namespace: ptr.To(apiv1.Namespace("default")),
			},
		}
	}

	if err := controller.NewGatewayClassProvisioner(mgr, gatewayControllerName, classConfigs); err != nil {
		cancel()
		return nil, err
	}

	poolCfg := &controller.InferencePoolConfig{
		Mgr:            mgr,
		ControllerName: gatewayControllerName,
		InferenceExt:   inferenceExt,
	}
	if err := controller.NewBaseInferencePoolController(parentCtx, poolCfg, &gwCfg, nil); err != nil {
		cancel()
		return nil, err
	}

	go func() {
		defer GinkgoRecover()
		kubeconfig = generateKubeConfiguration(cfg)
		mgr.GetLogger().Info("starting manager", "kubeconfig", kubeconfig)
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred())
	}()

	return func() {
		cancel()
		kubeClient.Shutdown()
	}, nil
}

func newCommonCols(ctx context.Context, kubeClient kube.Client) *collections.CommonCollections {
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	cli, err := versioned.NewForConfig(cfg)
	if err != nil {
		Expect(err).ToNot(HaveOccurred())
	}

	settings, err := settings.BuildSettings()
	if err != nil {
		Expect(err).ToNot(HaveOccurred())
	}
	commoncol, err := collections.NewCommonCollections(ctx, krtopts, kubeClient, cli, nil, gatewayControllerName, *settings)
	if err != nil {
		Expect(err).ToNot(HaveOccurred())
	}

	plugins := registry.Plugins(ctx, commoncol, wellknown.DefaultWaypointClassName, *settings)
	plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))
	extensions := registry.MergePlugins(plugins...)

	commoncol.InitPlugins(ctx, extensions, *settings)
	kubeClient.RunAndWait(ctx.Done())
	return commoncol
}

// Controller routines all in waiting state
var allowedRunningGoroutines = []types.GomegaMatcher{
	gleak.IgnoringTopFunction("sync.runtime_notifyListWait [sync.Cond.Wait]"),
	gleak.IgnoringTopFunction("istio.io/istio/pkg/kube/krt.(*processorListener[...]).run [select]"),
	gleak.IgnoringTopFunction("istio.io/istio/pkg/kube/krt.(*processorListener[...]).pop [select]"),
	gleak.IgnoringTopFunction(`istio.io/istio/pkg/queue.(*queueImpl).Run.func2 [chan receive]`),
}

func waitForGoroutinesToFinish(monitor *assertions.GoRoutineMonitor) {
	monitor.AssertNoLeaks(&assertions.AssertNoLeaksArgs{
		AllowedRoutines: allowedRunningGoroutines,
	})
}
