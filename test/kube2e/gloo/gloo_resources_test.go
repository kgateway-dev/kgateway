package gloo_test

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	kubecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

var _ = Describe("GlooResourcesTest", func() {
	const (
		gatewayProxy = defaults.GatewayProxyName
		gatewayPort  = int(80)
	)
	var (
		ctx        context.Context
		cfg        *rest.Config
		kubeClient kubernetes.Interface
		cache      kube.SharedCache

		gatewayClient           gatewayv1.GatewayClient
		httpGatewayClient       gatewayv1.MatchableHttpGatewayClient
		virtualServiceClient    gatewayv1.VirtualServiceClient
		routeTableClient        gatewayv1.RouteTableClient
		virtualHostOptionClient gatewayv1.VirtualHostOptionClient
		routeOptionClient       gatewayv1.RouteOptionClient
		upstreamGroupClient     gloov1.UpstreamGroupClient
		upstreamClient          gloov1.UpstreamClient
		proxyClient             gloov1.ProxyClient
	)
	BeforeEach(func() {
		var err error
		err = os.Setenv(statusutils.PodNamespaceEnvName, namespace)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel = context.WithCancel(context.Background())

		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kubeClient, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		cache = kube.NewKubeCache(ctx)
		gatewayClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.GatewayCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		httpGatewayClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.MatchableHttpGatewayCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		virtualServiceClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.VirtualServiceCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		routeTableClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.RouteTableCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		upstreamGroupClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.UpstreamGroupCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		upstreamClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.UpstreamCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		proxyClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.ProxyCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		virtualHostOptionClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.VirtualHostOptionCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		routeOptionClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.RouteOptionCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		gatewayClient, err = gatewayv1.NewGatewayClient(ctx, gatewayClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = gatewayClient.Register()
		Expect(err).NotTo(HaveOccurred())

		httpGatewayClient, err = gatewayv1.NewMatchableHttpGatewayClient(ctx, httpGatewayClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = httpGatewayClient.Register()
		Expect(err).NotTo(HaveOccurred())

		virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(ctx, virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualServiceClient.Register()
		Expect(err).NotTo(HaveOccurred())

		routeTableClient, err = gatewayv1.NewRouteTableClient(ctx, routeTableClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = routeTableClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamGroupClient, err = gloov1.NewUpstreamGroupClient(ctx, upstreamGroupClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamGroupClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamClient, err = gloov1.NewUpstreamClient(ctx, upstreamClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamClient.Register()
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err = gloov1.NewProxyClient(ctx, proxyClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = proxyClient.Register()
		Expect(err).NotTo(HaveOccurred())

		virtualHostOptionClient, err = gatewayv1.NewVirtualHostOptionClient(ctx, virtualHostOptionClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualHostOptionClient.Register()
		Expect(err).NotTo(HaveOccurred())

		routeOptionClient, err = gatewayv1.NewRouteOptionClient(ctx, routeOptionClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = routeOptionClient.Register()
		Expect(err).NotTo(HaveOccurred())

		kubeCoreCache, err := kubecache.NewKubeCoreCache(ctx, kubeClient)
		Expect(err).NotTo(HaveOccurred())
		_ = service.NewServiceClient(kubeClient, kubeCoreCache)

		_ = gloostatusutils.GetStatusClientForNamespace(testHelper.InstallNamespace)
	})
	Context("rotating secrets on upstream sslConfig", func() {
		const (
			tlsName          = "tls-server"
			defaultNamespace = "default"
			podServicePort   = 8080
			secretName       = "api-ssl"
			systemNamespace  = "gloo-system"
		)

		BeforeEach(func() {
			createTLSPod(kubeClient, upstreamClient, tlsName, defaultNamespace, systemNamespace, secretName, podServicePort)
			upstreamYAML := fmt.Sprintf(`
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: %[1]s-upstream
  namespace: %[2]s
spec:
  kube:
    selector:
      app: %[1]s
    serviceName: %[1]s
    serviceNamespace: default
    servicePort: %[3]d
  sslConfig:
    secretRef:
      name: %[4]s
      namespace: %[2]s
`, tlsName, systemNamespace, podServicePort, secretName)
			_, err := install.KubectlApplyOut([]byte(upstreamYAML))
			Expect(err).ToNot(HaveOccurred())
			virtualServiceYAML := fmt.Sprintf(`
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: %s
  namespace: %s
spec:
  virtualHost:
    domains:
    - "*"
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: %s-upstream 
            namespace: %s
`, tlsName, systemNamespace, tlsName, systemNamespace)
			_, err = install.KubectlApplyOut([]byte(virtualServiceYAML))
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should be able to rotate a secret referenced on a sslConfig on a kube upstream", func() {
			defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
			// wait for default gateway to be created so we can curl it
			Eventually(func() (*gatewayv1.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))
			rotateSecret(kubeClient, secretName, systemNamespace, false)
			// sleep for rotating the first secret
			time.Sleep(2 * time.Second)
			// 22 seconds between rotation with the offset added as well
			secondsForCurling := 22 * time.Second
			offset := 2 * time.Second
			timeForCurling := 5 * time.Second
			curlPod := func() bool {
				res, err := testHelper.Curl(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/hello",
					Method:            "GET",
					Host:              defaults.GatewayProxyName,
					Service:           defaults.GatewayProxyName,
					Port:              gatewayPort,
					ConnectionTimeout: 1,
					WithoutStats:      true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(ContainSubstring("Hello, world!"))
				// rotate and then wait for more than 15 seconds
				rotateSecret(kubeClient, secretName, systemNamespace, false)
				return true
			}
			timeInBetweenRotation := secondsForCurling + timeForCurling + offset
			// test it 10 time to ensure that the rotation works properly
			ConsistentlyWithOffset(2, curlPod, timeInBetweenRotation*10, timeInBetweenRotation).Should(Equal(true))
		})
	})
})

func createTLSPod(kubeClient kubernetes.Interface, upstreamClient gloov1.UpstreamClient, tlsName, defaultNamespace, secretNamespace, secretName string, podServicePort int) {
	tlsPodSchema := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsName,
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"app": tlsName,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: tlsName,
			Containers: []corev1.Container{{
				Name:  "example-tls-server",
				Image: "docker.io/soloio/example-tls-server:latest",
				Ports: []corev1.ContainerPort{{
					Name:          "http",
					ContainerPort: int32(podServicePort),
				}},
			}},
		},
	}
	tlsServiceSchema := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsName,
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"app": tlsName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       int32(podServicePort),
				TargetPort: intstr.FromInt(podServicePort),
			}},
			Selector: map[string]string{"app": tlsName},
		},
	}
	tlsServiceAccountSchema := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsName,
			Namespace: defaultNamespace,
		},
	}

	_, err := kubeClient.CoreV1().ServiceAccounts(defaultNamespace).Create(ctx, &tlsServiceAccountSchema, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.CoreV1().Services(defaultNamespace).Create(ctx, &tlsServiceSchema, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.CoreV1().Pods(defaultNamespace).Create(ctx, &tlsPodSchema, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
	rotateSecret(kubeClient, secretName, secretNamespace, true)
	// sleep to allow the pod to be registered
	time.Sleep(10 * time.Second)
}

func rotateSecret(kubeClient kubernetes.Interface, secretName, secretNamespace string, create bool) {
	crt, crtKey := helpers.GetCerts(helpers.Params{
		Hosts: "localhost",
		IsCA:  true,
	})
	tlsUpstreamSslConfigSecret := corev1.Secret{
		Type: corev1.SecretTypeTLS,
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNamespace,
		},
		// insert the base64 values and lets see that it just works
		Data: map[string][]byte{
			"tls.crt": []byte(crt),
			"tls.key": []byte(crtKey),
		},
	}
	if create {
		_, err := kubeClient.CoreV1().Secrets(secretNamespace).Create(ctx, &tlsUpstreamSslConfigSecret, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	} else {
		_, err := kubeClient.CoreV1().Secrets(secretNamespace).Update(ctx, &tlsUpstreamSslConfigSecret, metav1.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}
