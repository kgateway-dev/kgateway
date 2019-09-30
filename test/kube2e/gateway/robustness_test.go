package gateway_test

import (
	"fmt"
	"sort"
	"time"

	"github.com/solo-io/gloo/projects/gateway/pkg/services/k8sadmisssion"

	"github.com/solo-io/gloo/projects/gateway/pkg/translator"

	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/go-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/labels"

	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Robustness tests", func() {

	const (
		gatewayProxy = translator.GatewayProxyName
		gatewayPort  = int(80)
	)

	var (
		ctx       context.Context
		cancel    context.CancelFunc
		cfg       *rest.Config
		cache     kube.SharedCache
		namespace string

		kubeClient           kubernetes.Interface
		proxyClient          gloov1.ProxyClient
		virtualServiceClient gatewayv1.VirtualServiceClient

		appName        = "echo-app-for-robustness-test"
		appDeployment  *appsv1.Deployment
		appService     *corev1.Service
		virtualService *gatewayv1.VirtualService

		err error
	)

	var _ = BeforeEach(StartTestHelper)

	var _ = AfterEach(TearDownTestHelper)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		namespace = testHelper.InstallNamespace

		var err error
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		kubeClient, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		cache = kube.NewKubeCache(ctx)
		virtualServiceClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.VirtualServiceCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		proxyClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.ProxyCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualServiceClient.Register()
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err = gloov1.NewProxyClient(proxyClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = proxyClient.Register()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
	})

	It("updates Envoy endpoints even if proxy is rejected", func() {

		By("create a deployment and a matching service")
		appDeployment, appService, err = createDeploymentAndService(kubeClient, namespace, appName)
		Expect(err).NotTo(HaveOccurred())

		By("create a virtual service routing to the service")
		virtualService, err = virtualServiceClient.Write(&gatewayv1.VirtualService{
			Metadata: core.Metadata{
				Name:      "echo-vs",
				Namespace: namespace,
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Domains: []string{"*"},
				Routes: []*gatewayv1.Route{
					{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Prefix{
								Prefix: "/1",
							},
						},
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Kube{
											Kube: &gloov1.KubernetesServiceDestination{
												Ref: core.ResourceRef{
													Namespace: appService.Namespace,
													Name:      appService.Name,
												},
												Port: 5678,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		By("wait for proxy to be accepted")
		Eventually(func() error {
			proxy, err := proxyClient.Read(namespace, translator.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return err
			}
			if proxy.Status.State == core.Status_Accepted {
				return nil
			}
			return errors.Errorf("waiting for proxy to be accepted, but status is %v", proxy.Status)
		}, 60*time.Second, 1*time.Second).Should(BeNil())

		By("verify that we can route to the service")
		time.Sleep(1 * time.Second) // sleep a sec to save us some unnecessary polling
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "http",
			Path:              "/1",
			Method:            "GET",
			Host:              gatewayProxy,
			Service:           gatewayProxy,
			Port:              gatewayPort,
			ConnectionTimeout: 1,
			WithoutStats:      true,
		}, expectedResponse(appName), 1, 30*time.Second, 1*time.Second)

		By("add an invalid route to the virtual service")
		virtualService, err = virtualServiceClient.Read(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.ReadOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		virtualService.VirtualHost.Routes = append(virtualService.VirtualHost.Routes, &gatewayv1.Route{
			Matcher: &gloov1.Matcher{
				PathSpecifier: &gloov1.Matcher_Prefix{
					Prefix: "/3",
				},
			},
			Action: &gatewayv1.Route_RouteAction{
				RouteAction: &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_Single{
						Single: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Kube{
								Kube: &gloov1.KubernetesServiceDestination{
									Ref: core.ResourceRef{
										Namespace: namespace,
										Name:      "non-existent-svc",
									},
									Port: 1234,
								},
							},
						},
					},
				},
			},
		})

		// required to prevent gateway webhook from rejecting
		virtualService.Metadata.Annotations = map[string]string{k8sadmisssion.SkipValidationKey: k8sadmisssion.SkipValidationValue}

		virtualService, err = virtualServiceClient.Write(virtualService, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())

		By("wait for proxy to enter warning state")
		Eventually(func() error {
			proxy, err := proxyClient.Read(namespace, translator.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return err
			}
			if proxy.Status.State == core.Status_Warning {
				return nil
			}
			return errors.Errorf("waiting for proxy to be warning, but status is %v", proxy.Status)
		}, 20*time.Second, 1*time.Second).Should(BeNil())

		By("force an update of the service endpoints")
		initialIps := endpointsFor(kubeClient, appService)
		// Scale to 0 and back to 1 replicas until we have a different IP for the endpoint
		Eventually(func() []string {
			scaleDeploymentTo(kubeClient, appDeployment, 0)
			scaleDeploymentTo(kubeClient, appDeployment, 1)
			newIps := endpointsFor(kubeClient, appService)
			return newIps
		}, 20*time.Second, 1*time.Second).Should(And(
			HaveLen(len(initialIps)),
			Not(BeEquivalentTo(initialIps)),
		))

		By("verify that the new endpoints have been propagated to Envoy")
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "http",
			Path:              "/1",
			Method:            "GET",
			Host:              gatewayProxy,
			Service:           gatewayProxy,
			Port:              gatewayPort,
			ConnectionTimeout: 1,
			WithoutStats:      true,
		}, expectedResponse(appName), 1, 30*time.Second, 1*time.Second)
	})
})

func expectedResponse(appName string) string {
	return fmt.Sprintf("Hello from %s!", appName)
}

func createDeploymentAndService(kubeClient kubernetes.Interface, namespace, appName string) (
	*appsv1.Deployment, *corev1.Service, error,
) {
	deployment, err := kubeClient.AppsV1().Deployments(namespace).Create(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   appName,
			Labels: map[string]string{"app": appName},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointerToInt32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": appName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": appName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "http-echo",
						Image: "hashicorp/http-echo",
						Args:  []string{fmt.Sprintf("-text=%s", expectedResponse(appName))},
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: 5678,
						}},
					}},
					// important, otherwise termination lasts 30 seconds!
					TerminationGracePeriodSeconds: pointerToInt64(0),
				},
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	service, err := kubeClient.CoreV1().Services(namespace).Create(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   appName,
			Labels: map[string]string{"app": appName},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": appName},
			Ports: []corev1.ServicePort{{
				Name: "http",
				Port: 5678,
			}},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	return deployment, service, nil
}

func pointerToInt32(value int32) *int32 {
	return &value
}
func pointerToInt64(value int64) *int64 {
	return &value
}

func endpointsFor(kubeClient kubernetes.Interface, svc *corev1.Service) []string {
	var endpoints *corev1.EndpointsList
	listOpts := metav1.ListOptions{LabelSelector: labels.SelectorFromSet(svc.Spec.Selector).String()}
	endpoints, err := kubeClient.CoreV1().Endpoints(svc.Namespace).List(listOpts)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	var ips []string
	for _, endpoint := range endpoints.Items {
		for _, subset := range endpoint.Subsets {
			for _, address := range subset.Addresses {
				ips = append(ips, address.IP)
			}
		}
	}

	sort.Strings(ips)
	return ips
}

func scaleDeploymentTo(kubeClient kubernetes.Interface, deployment *appsv1.Deployment, replicas int32) {
	// Do this in an Eventually block, as the update sometimes fails due to concurrent modification
	EventuallyWithOffset(1, func() error {
		// Get deployment
		deployment, err := kubeClient.AppsV1().Deployments(deployment.Namespace).Get(deployment.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Scale it
		deployment.Spec.Replicas = pointerToInt32(replicas)
		deployment, err = kubeClient.AppsV1().Deployments(deployment.Namespace).Update(deployment)
		if err != nil {
			return err
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(BeNil())

	// Wait for expected running pod number
	EventuallyWithOffset(1, func() error {
		listOpts := metav1.ListOptions{LabelSelector: labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels).String()}
		pods, err := kubeClient.CoreV1().Pods(deployment.Namespace).List(listOpts)
		if err != nil {
			return err
		}
		if len(pods.Items) == int(replicas) {
			for _, pod := range pods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					return errors.Errorf("expected pod %v to be %s but was %s", pod.Name, corev1.PodRunning, pod.Status.Phase)
				}
			}
			return nil
		}
		return errors.Errorf("expected %d pods but found %d", replicas, len(pods.Items))
	}, 60*time.Second, 1*time.Second).Should(BeNil())
}
