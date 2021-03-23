package gateway_test

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"sort"
	"time"

	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	skerrors "github.com/solo-io/solo-kit/pkg/errors"

	"github.com/solo-io/gloo/projects/gateway/pkg/services/k8sadmisssion"

	testutils "github.com/solo-io/k8s-utils/testutils/kube"

	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/labels"

	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const (
	configDumpPath = "http://localhost:19000/config_dump"
	clustersPath   = "http://localhost:19000/clusters"
	kubeCtx        = ""
)

var _ = Describe("Robustness tests", func() {

	const (
		gatewayProxy = defaults.GatewayProxyName
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
		upstreamClient       gloov1.UpstreamClient

		appName        = "echo-app-for-robustness-test"
		appDeployment  *appsv1.Deployment
		appService     *corev1.Service
		virtualService *gatewayv1.VirtualService

		err error
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		namespace = testHelper.InstallNamespace

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
		upstreamClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.UpstreamCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(ctx, virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualServiceClient.Register()
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err = gloov1.NewProxyClient(ctx, proxyClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = proxyClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamClient, err = gloov1.NewUpstreamClient(ctx, upstreamClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamClient.Register()
		Expect(err).NotTo(HaveOccurred())

		appDeployment, appService, err = createDeploymentAndService(kubeClient, namespace, appName)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if virtualService != nil {
			_ = virtualServiceClient.Delete(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.DeleteOpts{Ctx: ctx, IgnoreNotExist: true})

			Eventually(func() bool {
				_, err := virtualServiceClient.Read(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.ReadOpts{Ctx: ctx})
				if err != nil && skerrors.IsNotExist(err) {
					return true
				}
				return false
			}, "15s", "0.5s").Should(BeTrue())
		}
		if appDeployment != nil {
			err := kubeClient.AppsV1().Deployments(namespace).Delete(ctx, appDeployment.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				deployments, err := kubeClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": appName}).String()})
				Expect(err).NotTo(HaveOccurred())
				return len(deployments.Items) == 0
			}, "15s", "0.5s").Should(BeTrue())
		}
		if appService != nil {
			err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, appService.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				services, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": appName}).String()})
				Expect(err).NotTo(HaveOccurred())
				return len(services.Items) == 0
			}, "15s", "0.5s").Should(BeTrue())
		}

		cancel()
	})

	It("updates Envoy endpoints even if proxy is rejected", func() {

		By("create a virtual service routing to the service")
		virtualService, err = virtualServiceClient.Write(&gatewayv1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "echo-vs",
				Namespace: namespace,
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Domains: []string{"*"},
				Routes: []*gatewayv1.Route{
					{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/1",
							},
						}},
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Kube{
											Kube: &gloov1.KubernetesServiceDestination{
												Ref: &core.ResourceRef{
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
			proxy, err := proxyClient.Read(namespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return err
			}
			if proxy.GetStatus().GetState() == core.Status_Accepted {
				return nil
			}
			return eris.Errorf("waiting for proxy to be accepted, but status is %v", proxy.Status)
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
			Matchers: []*matchers.Matcher{{
				PathSpecifier: &matchers.Matcher_Prefix{
					Prefix: "/3",
				},
			}},
			Action: &gatewayv1.Route_RouteAction{
				RouteAction: &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_Single{
						Single: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Kube{
								Kube: &gloov1.KubernetesServiceDestination{
									Ref: &core.ResourceRef{
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

		virtualServiceReconciler := gatewayv1.NewVirtualServiceReconciler(virtualServiceClient)
		err = virtualServiceReconciler.Reconcile(testHelper.InstallNamespace, gatewayv1.VirtualServiceList{virtualService}, nil, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())

		By("wait for proxy to enter warning state")
		Eventually(func() error {
			proxy, err := proxyClient.Read(namespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return err
			}
			if proxy.GetStatus().GetState() == core.Status_Warning {
				return nil
			}
			return eris.Errorf("waiting for proxy to be warning, but status is %v", proxy.Status)
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

	It("updates Envoy endpoints even if proxy is invalid and snapshot cache is reset", func() {

		upstream := &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "test",
				Namespace: "gloo-system",
			},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &static_plugin_gloo.UpstreamSpec{
					Hosts: []*static_plugin_gloo.Host{{
						Addr: "localhost",
						Port: 1234,
					}},
				},
			},
		}
		_, err := upstreamClient.Write(upstream, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
		Expect(err).ToNot(HaveOccurred())

		gatewayProxyPodName := testutils.FindPodNameByLabel(cfg, ctx, "gloo-system", "gloo=gateway-proxy")
		// We should consistently be able to modify upstreams
		Eventually(func() error {
			// Modify the upstream
			us, err := upstreamClient.Read(namespace, upstream.Metadata.Name, clients.ReadOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())
			if us.Status.State == core.Status_Accepted {
				return nil
			}
			return eris.Errorf("waiting for proxy to be accepted, but status is %v", us.Status)
		}, "3m", "5s").Should(BeNil())

		By("create a virtual service routing to the service")
		virtualService, err = virtualServiceClient.Write(&gatewayv1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "echo-vs",
				Namespace: namespace,
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Domains: []string{"*"},
				Routes: []*gatewayv1.Route{
					{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/1",
							},
						}},
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Kube{
											Kube: &gloov1.KubernetesServiceDestination{
												Ref: &core.ResourceRef{
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
			proxy, err := proxyClient.Read(namespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return err
			}
			if proxy.GetStatus().GetState() == core.Status_Accepted {
				return nil
			}
			return eris.Errorf("waiting for proxy to be accepted, but status is %v", proxy.Status)
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

		// Break config
		By("add conflicting domains to the virtual service")
		virtualService, err = virtualServiceClient.Read(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.ReadOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// conflicting domains
		virtualService.VirtualHost.Domains = []string{"*", "*"}

		// required to prevent gateway webhook from rejecting
		virtualService.Metadata.Annotations = map[string]string{k8sadmisssion.SkipValidationKey: k8sadmisssion.SkipValidationValue}

		virtualServiceReconciler := gatewayv1.NewVirtualServiceReconciler(virtualServiceClient)
		err = virtualServiceReconciler.Reconcile(testHelper.InstallNamespace, gatewayv1.VirtualServiceList{virtualService}, nil, clients.ListOpts{})
		Expect(err).NotTo(HaveOccurred())

		By("wait for proxy to enter rejected state")
		Eventually(func() error {
			proxy, err := proxyClient.Read(namespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return err
			}
			if proxy.GetStatus().GetState() == core.Status_Rejected {
				return nil
			}
			return eris.Errorf("waiting for proxy to be rejected, but status is %v", proxy.Status)
		}, 20*time.Second, 1*time.Second).Should(BeNil())

		By("reset snapshot cache")
		pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"gloo": "gloo"}).String()})
		Expect(err).ToNot(HaveOccurred())
		Expect(len(pods.Items)).To(Equal(1))
		oldGlooPod := pods.Items[0]

		err = kubeClient.CoreV1().Pods(namespace).Delete(ctx, oldGlooPod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
		Expect(err).ToNot(HaveOccurred())

		// check deleted and recreated
		Eventually(func() bool {
			pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"gloo": "gloo"}).String()})
			Expect(err).ToNot(HaveOccurred())
			if len(pods.Items) > 0 {
				// new pod name will not match old gloo pod
				return pods.Items[0].Name == oldGlooPod.Name
			}
			return true
		}, 80*time.Second, 2*time.Second).Should(BeFalse())

		By("force an update of the service endpoints")
		initialIps := endpointsFor(kubeClient, appService)
		var newIps []string
		// Scale to 0 and back to 1 replicas until we have a different IP for the endpoint
		Eventually(func() []string {
			scaleDeploymentTo(kubeClient, appDeployment, 0)
			scaleDeploymentTo(kubeClient, appDeployment, 1)
			newIps = endpointsFor(kubeClient, appService)
			return newIps
		}, 20*time.Second, 1*time.Second).Should(And(
			HaveLen(len(initialIps)),
			Not(BeEquivalentTo(initialIps)),
		))

		By("verify that the new endpoints have been propagated to Envoy")
		Eventually(func() bool {
			clusters := testutils.CurlWithEphemeralPod(ctx, ioutil.Discard, kubeCtx, "gloo-system", gatewayProxyPodName, clustersPath)
			testOldClusterEndpoints := regexp.MustCompile(initialIps[0] + ":")
			oldEndpointMatches := testOldClusterEndpoints.FindAllStringIndex(clusters, -1)
			testNewClusterEndpoints := regexp.MustCompile(newIps[0] + ":")
			newEndpointMatches := testNewClusterEndpoints.FindAllStringIndex(clusters, -1)
			fmt.Println(fmt.Sprintf("Number of cluster stats for old endpoint on clusters page: %d", len(oldEndpointMatches)))
			fmt.Println(fmt.Sprintf("Number of cluster stats for new endpoint on clusters page: %d", len(newEndpointMatches)))
			return len(oldEndpointMatches) == 0 && len(newEndpointMatches) > 0
		}, 20*time.Second, 1*time.Second).Should(BeTrue())

	})
})

func expectedResponse(appName string) string {
	return fmt.Sprintf("Hello from %s!", appName)
}

func createDeploymentAndService(kubeClient kubernetes.Interface, namespace, appName string) (
	*appsv1.Deployment, *corev1.Service, error,
) {
	deployment, err := kubeClient.AppsV1().Deployments(namespace).Create(ctx, &appsv1.Deployment{
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
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, err
	}

	service, err := kubeClient.CoreV1().Services(namespace).Create(ctx, &corev1.Service{
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
	}, metav1.CreateOptions{})
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
	endpoints, err := kubeClient.CoreV1().Endpoints(svc.Namespace).List(ctx, listOpts)
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
		deployment, err := kubeClient.AppsV1().Deployments(deployment.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Scale it
		deployment.Spec.Replicas = pointerToInt32(replicas)
		deployment, err = kubeClient.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(BeNil())

	// Wait for expected running pod number
	EventuallyWithOffset(1, func() error {
		listOpts := metav1.ListOptions{LabelSelector: labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels).String()}
		pods, err := kubeClient.CoreV1().Pods(deployment.Namespace).List(ctx, listOpts)
		if err != nil {
			return err
		}
		if len(pods.Items) == int(replicas) {
			for _, pod := range pods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					return eris.Errorf("expected pod %v to be %s but was %s", pod.Name, corev1.PodRunning, pod.Status.Phase)
				}
			}
			return nil
		}
		return eris.Errorf("expected %d pods but found %d", replicas, len(pods.Items))
	}, 60*time.Second, 1*time.Second).Should(BeNil())
}
