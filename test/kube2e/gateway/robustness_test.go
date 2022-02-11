package gateway_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"

	defaults2 "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/cliutils"

	"github.com/rotisserie/eris"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gateway/pkg/services/k8sadmission"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"

	testutils "github.com/solo-io/k8s-utils/testutils/kube"

	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/labels"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Robustness tests", func() {

	// These tests are used to validate our Endpoint Disovery Service (EDS) functionality
	// Historically, we had an EDS Test Suite (https://github.com/solo-io/gloo/tree/197272444efae0e6649c798997d6efa94bb7a8d9/test/kube2e/eds)
	// however, those tests often flaked, and the purpose of those tests was to validate what these
	// tests already do: that endpoints are updated and sent to Envoy successfully.
	// Therefore, we opted to collapse that Test Suite into this file. If in the future there are a larger set
	// of tests, we can evaluate re-opening that Test Suite.

	const (
		gatewayProxy = defaults.GatewayProxyName
		gatewayPort  = int(80)
	)

	var (
		ctx    context.Context
		cancel context.CancelFunc
		cfg    *rest.Config
		cache  kube.SharedCache

		kubeClient           kubernetes.Interface
		proxyClient          gloov1.ProxyClient
		virtualServiceClient gatewayv1.VirtualServiceClient
		statusClient         resources.StatusClient

		appName        = "echo-app-for-robustness-test"
		appDeployment  *appsv1.Deployment
		appService     *corev1.Service
		virtualService *gatewayv1.VirtualService

		err error
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

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

		virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(ctx, virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualServiceClient.Register()
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err = gloov1.NewProxyClient(ctx, proxyClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = proxyClient.Register()
		Expect(err).NotTo(HaveOccurred())

		statusClient = gloostatusutils.GetStatusClientForNamespace(namespace)

		appDeployment, appService, err = createEchoDeploymentAndService(kubeClient, testHelper.InstallNamespace, appName)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = kubeClient.AppsV1().Deployments(testHelper.InstallNamespace).Delete(ctx, appDeployment.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
		Eventually(func() bool {
			deployments, err := kubeClient.AppsV1().Deployments(testHelper.InstallNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": appName}).String()})
			Expect(err).NotTo(HaveOccurred())
			return len(deployments.Items) == 0
		}, "15s", "0.5s").Should(BeTrue())

		_ = kubeClient.CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, appService.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
		Eventually(func() bool {
			services, err := kubeClient.CoreV1().Services(testHelper.InstallNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": appName}).String()})
			Expect(err).NotTo(HaveOccurred())
			return len(services.Items) == 0
		}, "15s", "0.5s").Should(BeTrue())

		cancel()
	})

	Context("Updates Envoy endpoints, even if proxy is invalid", func() {

		BeforeEach(func() {
			virtualService = &gatewayv1.VirtualService{
				Metadata: &core.Metadata{
					Name:      "echo-vs",
					Namespace: testHelper.InstallNamespace,
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
			}
		})

		JustBeforeEach(func() {
			_, writeErr := virtualServiceClient.Write(virtualService, clients.WriteOpts{Ctx: ctx})
			Expect(writeErr).NotTo(HaveOccurred())

			// Wait for the proxy to be accepted
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return proxyClient.Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			}, 60*time.Second, 1*time.Second)

			// Ensure we can route to the service
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/1",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, expectedResponse(appName), 1, 90*time.Second, 1*time.Second)
		})

		AfterEach(func() {
			_ = virtualServiceClient.Delete(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.DeleteOpts{Ctx: ctx, IgnoreNotExist: true})
			helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
				return virtualServiceClient.Read(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.ReadOpts{Ctx: ctx})
			}, "15s", "0.5s")
		})

		forceProxyIntoWarningState := func(virtualService *gatewayv1.VirtualService) {
			virtualService, err = virtualServiceClient.Read(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.ReadOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			// required to prevent gateway webhook from rejecting
			virtualService.Metadata.Annotations = map[string]string{k8sadmission.SkipValidationKey: k8sadmission.SkipValidationValue}

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
											Namespace: testHelper.InstallNamespace,
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

			virtualServiceReconciler := gatewayv1.NewVirtualServiceReconciler(virtualServiceClient, statusClient)
			err = virtualServiceReconciler.Reconcile(testHelper.InstallNamespace, gatewayv1.VirtualServiceList{virtualService}, nil, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())

			helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
				return proxyClient.Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			}, 20*time.Second, 1*time.Second)
		}

		It("works", func() {
			// we already verify that the initial curl works in the BeforeEach()
			By("force proxy into warning state")
			forceProxyIntoWarningState(virtualService)

			By("force an update of the service endpoints")
			initialEndpointIPs := endpointIPsForKubeService(kubeClient, appService)

			scaleDeploymentTo(kubeClient, appDeployment, 0)
			scaleDeploymentTo(kubeClient, appDeployment, 1)

			Eventually(func() []string {
				return endpointIPsForKubeService(kubeClient, appService)
			}, 20*time.Second, 1*time.Second).Should(And(
				HaveLen(len(initialEndpointIPs)),
				Not(BeEquivalentTo(initialEndpointIPs)),
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

		It("works, even when snapshot cache is reset", func() {
			By("force proxy into warning state")
			forceProxyIntoWarningState(virtualService)

			By("delete gloo pod, ensuring the snapshot cache is reset")
			pods, err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"gloo": "gloo"}).String()})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(pods.Items)).To(Equal(1))
			oldGlooPod := pods.Items[0]

			err = kubeClient.CoreV1().Pods(testHelper.InstallNamespace).Delete(ctx, oldGlooPod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				pods, err := kubeClient.CoreV1().Pods(testHelper.InstallNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"gloo": "gloo"}).String()})
				Expect(err).ToNot(HaveOccurred())
				if len(pods.Items) > 0 {
					// new pod name will not match old gloo pod
					return pods.Items[0].Name == oldGlooPod.Name
				}
				return true
			}, 80*time.Second, 2*time.Second).Should(BeFalse())

			By("force an update of the service endpoints")
			var initialEndpointIPs, newEndpointIPs []string
			initialEndpointIPs = endpointIPsForKubeService(kubeClient, appService)

			scaleDeploymentTo(kubeClient, appDeployment, 0)
			scaleDeploymentTo(kubeClient, appDeployment, 1)

			Eventually(func() []string {
				newEndpointIPs = endpointIPsForKubeService(kubeClient, appService)
				return newEndpointIPs
			}, 20*time.Second, 1*time.Second).Should(And(
				HaveLen(len(initialEndpointIPs)),
				Not(BeEquivalentTo(initialEndpointIPs)),
			))

			By("verify that the new endpoints have been propagated to Envoy")
			gatewayProxyPodName := testutils.FindPodNameByLabel(cfg, ctx, testHelper.InstallNamespace, "gloo=gateway-proxy")
			envoyClustersPath := "http://localhost:19000/clusters" // TODO - this should live in envoy test service
			Eventually(func() bool {
				clusters := testutils.CurlWithEphemeralPodStable(ctx, ioutil.Discard, "", testHelper.InstallNamespace, gatewayProxyPodName, envoyClustersPath)

				fmt.Println(fmt.Sprintf("initial endpoint ips %+v", initialEndpointIPs))

				testOldClusterEndpoints := regexp.MustCompile(initialEndpointIPs[0] + ":")
				oldEndpointMatches := testOldClusterEndpoints.FindAllStringIndex(clusters, -1)
				fmt.Println(fmt.Sprintf("Number of cluster stats for old endpoint on clusters page: %d", len(oldEndpointMatches)))

				fmt.Println(fmt.Sprintf("new endpoint ips %+v", newEndpointIPs))

				testNewClusterEndpoints := regexp.MustCompile(newEndpointIPs[0] + ":")

				if strings.Contains(clusters, "Error from server") {
					// make error clear in logs. e.g., running locally and ephemeral containers haven't been enabled
					fmt.Println(fmt.Sprintf("clusters: %+v", clusters))
				}

				newEndpointMatches := testNewClusterEndpoints.FindAllStringIndex(clusters, -1)
				fmt.Println(fmt.Sprintf("Number of cluster stats for new endpoint on clusters page: %d", len(newEndpointMatches)))

				return len(oldEndpointMatches) == 0 && len(newEndpointMatches) > 0
			}, 60*time.Second, 1*time.Second).Should(BeTrue())

		})

		Context("xds-relay", func() {
			const (
				xdsRelayReplicas = 5
				envoyReplicas    = 8
			)
			var (
				xdsRelayDeployment = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "xds-relay",
						Labels:    map[string]string{"app": "xds-relay"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: pointerToInt32(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "xds-relay"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "xds-relay"},
							},
						},
					},
				}
				envoyDeployment = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gloo-system",
						Name:      "gateway-proxy",
						Labels:    map[string]string{"gloo": "gateway-proxy"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: pointerToInt32(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"gloo": "gateway-proxy"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"gloo": "gateway-proxy"},
							},
						},
					},
				}
				glooDeployment = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gloo-system",
						Name:      "gloo",
						Labels:    map[string]string{"gloo": "gloo"},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: pointerToInt32(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"gloo": "gloo"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"gloo": "gloo"},
							},
						},
					},
				}
				// labelSelector is a string map e.g. gloo=gateway-proxy
				findPodNamesByLabel = func(cfg *rest.Config, ctx context.Context, ns, labelSelector string) []string {
					clientset, err := kubernetes.NewForConfig(cfg)
					Expect(err).NotTo(HaveOccurred())
					pl, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
					Expect(err).NotTo(HaveOccurred())
					Expect(pl.Items).NotTo(BeEmpty())
					var names []string
					for _, item := range pl.Items {
						names = append(names, item.Name)
					}
					return names
				}
				findEchoAppClusterEndpoints = func(podName, expectedEndpoints string) int {
					clusters, portFwdCmd, err := cliutils.PortForwardGet(ctx, defaults2.GlooSystem, podName, "19000", "19000", true, "/clusters")
					if err != nil {
						fmt.Println(err)
					}
					if portFwdCmd.Process != nil {
						defer portFwdCmd.Process.Release()
						defer portFwdCmd.Process.Kill()
					}
					echoAppClusterEndpoints := regexp.MustCompile(fmt.Sprintf("\ngloo-system-echo-app-for-robustness-test-5678_gloo-system::%s:5678::", expectedEndpoints))
					matches := echoAppClusterEndpoints.FindAllStringIndex(clusters, -1)
					fmt.Println(fmt.Sprintf("Number of cluster stats for echo app (i.e., checking for endpoints) on clusters page: %d", len(matches)))
					return len(matches)
				}
			)

			It("works, even if gloo is scaled to zero and envoy is bounced", func() {

				if os.Getenv("USE_XDS_RELAY") != "true" {
					Skip("skipping test that only passes with xds relay enabled")
				}

				By("verify that the endpoints have been propagated to Envoy")
				// we already verify that the initial curl works in the BeforeEach()

				// scale to five replicas, envoy already connected to our initial xds-relay replica so the
				// other four will have stale caches
				scaleDeploymentTo(kubeClient, xdsRelayDeployment, xdsRelayReplicas)

				By("scale gloo to zero")
				scaleDeploymentTo(kubeClient, glooDeployment, 0)

				By("bounce envoy")
				scaleDeploymentTo(kubeClient, envoyDeployment, 0)
				// scale to eight replicas to ensure / maximize likelihood that at least one of the new envoys
				// will connect to an xds-relay replica with a cold cache. xds-relay should disconnect on the cache
				// miss and envoy will retry until it hits our xds-relay with the warm cache
				scaleDeploymentTo(kubeClient, envoyDeployment, envoyReplicas)

				// this asserts that at least one envoy has the correct endpoints
				By("verify that the endpoints have been propagated to Envoy by xds relay")
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

				envoyPodNames := findPodNamesByLabel(cfg, ctx, defaults2.GlooSystem, "gloo=gateway-proxy")
				Expect(envoyPodNames).To(HaveLen(envoyReplicas))

				initialEndpointIPs := endpointIPsForKubeService(kubeClient, appService)
				Expect(initialEndpointIPs).To(HaveLen(1))

				// this asserts that at all envoys have the correct endpoints.
				// envoy may need to retry until it hits xds relay with the warm cache, hence the 45s timeout.
				for _, envoyPodName := range envoyPodNames {
					fmt.Println(fmt.Sprintf("Checking for endpoints for %v", envoyPodName))
					Eventually(func() int {
						return findEchoAppClusterEndpoints(envoyPodName, initialEndpointIPs[0])
					}, "45s", "1s").Should(BeNumerically(">", 0))
				}

				By("reconnects to upstream gloo after scaling up, new endpoints are picked up")
				scaleDeploymentTo(kubeClient, glooDeployment, 1)

				By("force an update of the service endpoints")
				scaleDeploymentTo(kubeClient, appDeployment, 0)
				scaleDeploymentTo(kubeClient, appDeployment, 1)

				Eventually(func() []string {
					return endpointIPsForKubeService(kubeClient, appService)
				}, 20*time.Second, 1*time.Second).Should(And(
					HaveLen(len(initialEndpointIPs)),
					Not(BeEquivalentTo(initialEndpointIPs)),
				))

				// this asserts that at least one envoy has the correct endpoints
				By("verify that the new endpoints have been propagated to envoy by xds relay from gloo")
				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/1",
					Method:            "GET",
					Host:              gatewayProxy,
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1,
					WithoutStats:      true,
				}, expectedResponse(appName), 1, 60*time.Second, 1*time.Second)

				newEndpointIPs := endpointIPsForKubeService(kubeClient, appService)
				Expect(newEndpointIPs).To(HaveLen(1))

				// this asserts that at all envoys have the correct endpoints.
				// should be quicker than before, we already connected to the warm xds-relay
				// (and all xds relays should be able to reconnect to origin regardless).
				for _, envoyPodName := range envoyPodNames {
					fmt.Println(fmt.Sprintf("Checking for endpoints for %v", envoyPodName))
					Eventually(func() int {
						return findEchoAppClusterEndpoints(envoyPodName, newEndpointIPs[0])
					}, "15s", "1s").Should(BeNumerically(">", 0))
				}
			})
		})
	})

})

func expectedResponse(appName string) string {
	return fmt.Sprintf("Hello from %s!", appName)
}

func createEchoDeploymentAndService(kubeClient kubernetes.Interface, namespace, appName string) (
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

func endpointIPsForKubeService(kubeClient kubernetes.Interface, svc *corev1.Service) []string {
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
