package controller_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/config"

	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	infextv1a1 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha1"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc

	kubeconfig string

	gwClasses = sets.New(gatewayClassName, altGatewayClassName)
)

const (
	gatewayClassName      = "clsname"
	altGatewayClassName   = "clsname-alt"
	gatewayControllerName = "kgateway.dev/kgateway"
)

func getAssetsDir() string {
	assets := ""
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// set default if not user provided
		out, err := exec.Command("sh", "-c", "make -sC $(dirname $(go env GOMOD))/internal/kgateway envtest-path").CombinedOutput()
		fmt.Fprintln(GinkgoWriter, "out:", string(out))
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		assets = strings.TrimSpace(string(out))
	}
	return assets
}

var _ = BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	// Create a scheme and add both Gateway and InferencePool types.
	scheme := schemes.GatewayScheme()
	err := infextv1a1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	// Required to deploy endpoint picker RBAC resources.
	err = rbacv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "crds"),
			filepath.Join("..", "..", "..", "install", "helm", "kgateway", "crds"),
		},
		ErrorIfCRDPathMissing: true,
		// set assets dir so we can run without the makefile
		BinaryAssetsDirectory: getAssetsDir(),
	}
	var err2 error
	cfg, err2 = testEnv.Start()
	Expect(err2).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgrOpts := ctrl.Options{
		Scheme: scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		Controller: config.Controller{
			// see https://github.com/kubernetes-sigs/controller-runtime/issues/2937
			// in short, our tests reuse the same name (reasonably so) and the controller-runtime
			// package does not reset the stack of controller names between tests, so we disable
			// the name validation here.
			SkipNameValidation: ptr.To(true),
		},
	}
	mgr, err := ctrl.NewManager(cfg, mgrOpts)
	Expect(err).ToNot(HaveOccurred())

	kubeconfig = generateKubeConfiguration(cfg)
	mgr.GetLogger().Info("starting manager", "kubeconfig", kubeconfig)

	// Start the Gateway controller.
	gwCfg := controller.GatewayConfig{
		Mgr:            mgr,
		ControllerName: gatewayControllerName,
		OurGateway: func(gw *apiv1.Gateway) bool {
			return gwClasses.Has(string(gw.Spec.GatewayClassName))
		},
		AutoProvision: true,
	}
	err = controller.NewBaseGatewayController(ctx, gwCfg)
	Expect(err).ToNot(HaveOccurred())

	// Start the inference pool controller.
	poolCfg := &controller.InferencePoolConfig{
		Mgr:            mgr,
		ControllerName: wellknown.GatewayControllerName,
		InferenceExt:   new(deployer.InferenceExtInfo),
	}
	err = controller.NewBaseInferencePoolController(ctx, poolCfg, &gwCfg)
	Expect(err).ToNot(HaveOccurred())

	// Create the default GatewayParameters and GatewayClass.
	err = k8sClient.Create(ctx, &v1alpha1.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wellknown.DefaultGatewayParametersName,
			Namespace: "default",
		},
		Spec: v1alpha1.GatewayParametersSpec{
			Kube: &v1alpha1.KubernetesProxyConfig{
				Service: &v1alpha1.Service{
					Type: ptr.To(corev1.ServiceTypeLoadBalancer),
				},
				Istio: &v1alpha1.IstioIntegration{},
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())

	for class := range gwClasses {
		err = k8sClient.Create(ctx, &apiv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: class,
			},
			Spec: apiv1.GatewayClassSpec{
				ControllerName: apiv1.GatewayController(gatewayControllerName),
				ParametersRef: &apiv1.ParametersReference{
					Group:     apiv1.Group(v1alpha1.GroupVersion.Group),
					Kind:      "GatewayParameters",
					Name:      wellknown.DefaultGatewayParametersName,
					Namespace: ptr.To(apiv1.Namespace("default")),
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
	}

	// Start the manager.
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
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

var _ = Describe("InferencePool controller", func() {
	const defaultNamespace = "default"

	It("should reconcile an InferencePool referenced by an HTTPRoute managed by our controller", func() {
		// Create a test Gateway that will be referenced by the HTTPRoute.
		testGw := &apiv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gateway",
				Namespace: defaultNamespace,
			},
			Spec: apiv1.GatewaySpec{
				GatewayClassName: gatewayClassName,
				Listeners: []apiv1.Listener{
					{
						Name:     "listener-1",
						Protocol: apiv1.HTTPProtocolType,
						Port:     80,
					},
				},
			},
		}
		err := k8sClient.Create(ctx, testGw)
		Expect(err).NotTo(HaveOccurred())

		// Create an HTTPRoute without a status.
		httpRoute := &apiv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route",
				Namespace: defaultNamespace,
			},
			Spec: apiv1.HTTPRouteSpec{
				Rules: []apiv1.HTTPRouteRule{
					{
						BackendRefs: []apiv1.HTTPBackendRef{
							{
								BackendRef: apiv1.BackendRef{
									BackendObjectReference: apiv1.BackendObjectReference{
										Group: ptr.To(apiv1.Group(infextv1a1.GroupVersion.Group)),
										Kind:  ptr.To(apiv1.Kind("InferencePool")),
										Name:  "pool1",
									},
								},
							},
						},
					},
				},
			},
		}
		err = k8sClient.Create(ctx, httpRoute)
		Expect(err).NotTo(HaveOccurred())

		// Now update the status to include a valid Parents field.
		httpRoute.Status = apiv1.HTTPRouteStatus{
			RouteStatus: apiv1.RouteStatus{
				Parents: []apiv1.RouteParentStatus{
					{
						ParentRef: apiv1.ParentReference{
							Group:     ptr.To(apiv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(apiv1.Kind("Gateway")),
							Name:      apiv1.ObjectName(testGw.Name),
							Namespace: ptr.To(apiv1.Namespace(defaultNamespace)),
						},
						ControllerName: gatewayControllerName,
					},
				},
			},
		}
		err = k8sClient.Status().Update(ctx, httpRoute)
		Expect(err).NotTo(HaveOccurred())

		// Create an InferencePool resource that is referenced by the HTTPRoute.
		pool := &infextv1a1.InferencePool{
			TypeMeta: metav1.TypeMeta{
				Kind:       "InferencePool",
				APIVersion: infextv1a1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pool1",
				Namespace: defaultNamespace,
				UID:       "pool-uid",
			},
			Spec: infextv1a1.InferencePoolSpec{
				Selector:         map[infextv1a1.LabelKey]infextv1a1.LabelValue{},
				TargetPortNumber: 1234,
				EndpointPickerConfig: infextv1a1.EndpointPickerConfig{
					ExtensionRef: &infextv1a1.Extension{
						ExtensionReference: infextv1a1.ExtensionReference{
							Name: "doesnt-matter",
						},
					},
				},
			},
		}
		err = k8sClient.Create(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		// The secondary watch on HTTPRoute should now trigger reconciliation of pool "pool1".
		// We expect the deployer to render and deploy an endpoint picker Deployment with name "pool1-endpoint-picker".
		expectedName := fmt.Sprintf("%s-endpoint-picker", pool.Name)
		var deploy appsv1.Deployment
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: defaultNamespace, Name: expectedName}, &deploy)
		}, "10s", "1s").Should(Succeed())
	})

	It("should ignore an InferencePool not referenced by any HTTPRoute", func() {
		// Create an InferencePool that is not referenced by any HTTPRoute.
		pool := &infextv1a1.InferencePool{
			TypeMeta: metav1.TypeMeta{
				Kind:       "InferencePool",
				APIVersion: infextv1a1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pool2",
				Namespace: defaultNamespace,
				UID:       "pool2-uid",
			},
			Spec: infextv1a1.InferencePoolSpec{
				Selector:         map[infextv1a1.LabelKey]infextv1a1.LabelValue{},
				TargetPortNumber: 1234,
				EndpointPickerConfig: infextv1a1.EndpointPickerConfig{
					ExtensionRef: &infextv1a1.Extension{
						ExtensionReference: infextv1a1.ExtensionReference{
							Name: "doesnt-matter",
						},
					},
				},
			},
		}
		err := k8sClient.Create(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		// Consistently check that no endpoint picker deployment is created.
		Consistently(func() error {
			var dep appsv1.Deployment
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: defaultNamespace, Name: fmt.Sprintf("%s-endpoint-picker", pool.Name)}, &dep)
		}, "5s", "1s").ShouldNot(Succeed())
	})
})
