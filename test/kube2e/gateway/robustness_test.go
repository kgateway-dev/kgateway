package gateway_test

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/rotisserie/eris"

	"github.com/solo-io/gloo/projects/gateway/pkg/services/k8sadmission"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"

	testutils "github.com/solo-io/k8s-utils/testutils/kube"

	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"k8s.io/apimachinery/pkg/labels"

	. "github.com/onsi/ginkgo/v2"
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

	// These tests are used to validate our Endpoint Discovery Service (EDS) functionality
	// Historically, we had an EDS Test Suite (https://github.com/solo-io/gloo/tree/197272444efae0e6649c798997d6efa94bb7a8d9/test/kube2e/eds)
	// however, those tests often flaked, and the purpose of those tests was to validate what these
	// tests already do: that endpoints are updated and sent to Envoy successfully.
	// Therefore, we opted to collapse that Test Suite into this file. If in the future there are a larger set
	// of tests, we can evaluate re-opening that Test Suite.

	var (
		appName       = "echo-app-for-robustness-test"
		appDeployment *appsv1.Deployment
		appService    *corev1.Service
		appVs         *gatewayv1.VirtualService

		err error

		envoyDeployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
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
				Namespace: namespace,
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

		glooResources *gloosnapshot.ApiSnapshot
	)

	BeforeEach(func() {
		appDeployment, appService, err = createEchoDeploymentAndService(resourceClientset.KubeClients(), testHelper.InstallNamespace, appName)
		Expect(err).NotTo(HaveOccurred())

		appVs = helpers.NewVirtualServiceBuilder().
			WithName(appName).
			WithNamespace(testHelper.InstallNamespace).
			WithLabel(kube2e.UniqueTestResourceLabel, uuid.New().String()).
			WithDomain("app").
			WithRoutePrefixMatcher("route", "/1").
			WithRouteActionToSingleDestination("route",
				&gloov1.Destination{
					DestinationType: &gloov1.Destination_Kube{
						Kube: &gloov1.KubernetesServiceDestination{
							Ref: &core.ResourceRef{
								Namespace: testHelper.InstallNamespace,
								Name:      appService.Name,
							},
							Port: 5678,
						},
					},
				}).
			Build()

		// The set of resources that these tests will generate
		glooResources = &gloosnapshot.ApiSnapshot{
			VirtualServices: gatewayv1.VirtualServiceList{
				appVs,
			},
		}
	})

	AfterEach(func() {
		_ = resourceClientset.KubeClients().AppsV1().Deployments(testHelper.InstallNamespace).Delete(ctx, appDeployment.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
		Eventually(func() bool {
			deployments, err := resourceClientset.KubeClients().AppsV1().Deployments(testHelper.InstallNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": appName}).String()})
			Expect(err).NotTo(HaveOccurred())
			return len(deployments.Items) == 0
		}, "15s", "0.5s").Should(BeTrue())

		_ = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, appService.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
		Eventually(func() bool {
			services, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": appName}).String()})
			Expect(err).NotTo(HaveOccurred())
			return len(services.Items) == 0
		}, "15s", "0.5s").Should(BeTrue())
	})

	JustBeforeEach(func() {
		err = snapshotWriter.WriteSnapshot(glooResources, clients.WriteOpts{
			Ctx:               ctx,
			OverwriteExisting: false,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	JustAfterEach(func() {
		err = snapshotWriter.DeleteSnapshot(glooResources, clients.DeleteOpts{
			Ctx:            ctx,
			IgnoreNotExist: true,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	// labelSelector is a string map e.g. gloo=gateway-proxy
	findPodNamesByLabel := func(ctx context.Context, ns, labelSelector string) []string {
		pl, err := resourceClientset.KubeClients().CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		Expect(err).NotTo(HaveOccurred())
		Expect(pl.Items).NotTo(BeEmpty())
		var names []string
		for _, item := range pl.Items {
			names = append(names, item.Name)
		}
		return names
	}

	Context("Updates Envoy endpoints, even if proxy is invalid", func() {

		forceProxyIntoWarningState := func(originalVs *gatewayv1.VirtualService) {
			virtualService, err := resourceClientset.VirtualServiceClient().Read(originalVs.GetMetadata().GetNamespace(), originalVs.GetMetadata().GetName(), clients.ReadOpts{Ctx: ctx})
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

			statusClient := gloostatusutils.GetStatusClientForNamespace(testHelper.InstallNamespace)
			virtualServiceReconciler := gatewayv1.NewVirtualServiceReconciler(resourceClientset.VirtualServiceClient(), statusClient)
			err = virtualServiceReconciler.Reconcile(testHelper.InstallNamespace, gatewayv1.VirtualServiceList{virtualService}, nil, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())

			helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
				return resourceClientset.ProxyClient().Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			})
		}

		It("works", func() {
			By("Ensure we can route to the service")
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/1",
				Method:            "GET",
				Host:              "app",
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, expectedResponse(appName), 1, 45*time.Second, 1*time.Second)

			By("force proxy into warning state")
			forceProxyIntoWarningState(appVs)

			var initialEndpointIPs []string
			Eventually(func() []string {
				initialEndpointIPs = endpointIPsForKubeService(resourceClientset.KubeClients(), appService)
				return initialEndpointIPs
			}, 20*time.Second, 1*time.Second).Should(Not(HaveLen(0)))

			By("force an update of the service endpoints")
			scaleDeploymentTo(resourceClientset.KubeClients(), appDeployment, 0)
			scaleDeploymentTo(resourceClientset.KubeClients(), appDeployment, 1)

			Eventually(func() []string {
				return endpointIPsForKubeService(resourceClientset.KubeClients(), appService)
			}, 20*time.Second, 1*time.Second).Should(And(
				HaveLen(len(initialEndpointIPs)),
				Not(BeEquivalentTo(initialEndpointIPs)),
			))

			By("verify that the new endpoints have been propagated to Envoy")
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/1",
				Method:            "GET",
				Host:              "app",
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, expectedResponse(appName), 1, 30*time.Second, 1*time.Second)
		})

		Context("works, even with deleted services", func() {

			var (
				appName2       = "echo-app-for-robustness-test2"
				appDeployment2 *appsv1.Deployment
				appService2    *corev1.Service
			)

			BeforeEach(func() {
				appDeployment2, appService2, err = createEchoDeploymentAndService(resourceClientset.KubeClients(), testHelper.InstallNamespace, appName2)
				Expect(err).NotTo(HaveOccurred())

				appVs.VirtualHost.Routes = append(appVs.VirtualHost.Routes, &gatewayv1.Route{
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/2",
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
												Name:      appService2.Name,
											},
											Port: 5678,
										},
									},
								},
							},
						},
					},
				})

				glooResources.VirtualServices = gatewayv1.VirtualServiceList{appVs}
			})

			AfterEach(func() {
				_ = resourceClientset.KubeClients().AppsV1().Deployments(testHelper.InstallNamespace).Delete(ctx, appDeployment2.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Eventually(func() bool {
					deployments, err := resourceClientset.KubeClients().AppsV1().Deployments(testHelper.InstallNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": appName2}).String()})
					Expect(err).NotTo(HaveOccurred())
					return len(deployments.Items) == 0
				}, "15s", "0.5s").Should(BeTrue())

				_ = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, appService2.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Eventually(func() bool {
					services, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": appName2}).String()})
					Expect(err).NotTo(HaveOccurred())
					return len(services.Items) == 0
				}, "15s", "0.5s").Should(BeTrue())
			})

			firstRouteCurlOpts := func() helper.CurlOpts {
				return helper.CurlOpts{
					Protocol:          "http",
					Path:              "/1",
					Method:            "GET",
					Host:              "app",
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1,
					WithoutStats:      true,
				}
			}

			secondRouteCurlOpts := func() helper.CurlOpts {
				return helper.CurlOpts{
					Protocol:          "http",
					Path:              "/2",
					Method:            "GET",
					Host:              "app",
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1,
					WithoutStats:      true,
				}
			}

			assertCanRouteSvc1AndSvc2 := func() {
				By("assert we can route to svc1 and to svc2")
				// Ensure we can route to the first service
				testHelper.CurlEventuallyShouldRespond(firstRouteCurlOpts(), expectedResponse(appName), 1, 45*time.Second, 1*time.Second)
				// Ensure we can route to the second service
				testHelper.CurlEventuallyShouldRespond(secondRouteCurlOpts(), expectedResponse(appName2), 1, 45*time.Second, 1*time.Second)
			}

			assertCanRouteSvc1NotSvc2 := func() {
				Eventually(func(g Gomega) {
					By("assert we can route to svc1")
					validRouteResponse := expectedResponse(appName)
					g.Eventually(func() (string, error) {
						return testHelper.Curl(firstRouteCurlOpts())
					}, "30s", "1s").Should(ContainSubstring(validRouteResponse))
					g.Consistently(func() (string, error) {
						return testHelper.Curl(firstRouteCurlOpts())
					}, "5s", "1s").Should(ContainSubstring(validRouteResponse))

					// can no longer route to appName2 since its k8s service has been removed
					// we have invalid route replacement enabled, so we should receive the response from the fallback cluster
					By("assert we can not route to svc2")
					invalidRouteResponse := "Gloo Gateway has invalid configuration."
					g.Eventually(func() (string, error) {
						return testHelper.Curl(secondRouteCurlOpts())
					}, "30s", "1s").Should(ContainSubstring(invalidRouteResponse))
					g.Consistently(func() (string, error) {
						return testHelper.Curl(secondRouteCurlOpts())
					}, "5s", "1s").Should(ContainSubstring(invalidRouteResponse))

				}, "30s")
			}

			It("works", func() {
				assertCanRouteSvc1AndSvc2()

				// Delete the k8s service behind the second echo app
				err = resourceClientset.KubeClients().CoreV1().Services(appService2.Namespace).Delete(ctx, appService2.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				assertCanRouteSvc1NotSvc2()
			})

			It("works, even with deleted services", func() {
				// Delete the k8s service behind the second echo app
				err = resourceClientset.KubeClients().CoreV1().Services(appService2.Namespace).Delete(ctx, appService2.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				assertCanRouteSvc1NotSvc2()

				// roll pods to ensure we are resilient to pod restarts
				By("bounce gloo and envoy")
				scaleDeploymentTo(resourceClientset.KubeClients(), glooDeployment, 0)
				scaleDeploymentTo(resourceClientset.KubeClients(), envoyDeployment, 0)
				scaleDeploymentTo(resourceClientset.KubeClients(), glooDeployment, 1)
				scaleDeploymentTo(resourceClientset.KubeClients(), envoyDeployment, 1)

				assertCanRouteSvc1NotSvc2()
			})

		})

		It("works, even when snapshot cache is reset", func() {
			By("Ensure we can route to the service")
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/1",
				Method:            "GET",
				Host:              "app",
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, expectedResponse(appName), 1, 45*time.Second, 1*time.Second)

			By("force proxy into warning state")
			forceProxyIntoWarningState(appVs)

			By("delete gloo pod, ensuring the snapshot cache is reset")
			scaleDeploymentTo(resourceClientset.KubeClients(), glooDeployment, 0)
			scaleDeploymentTo(resourceClientset.KubeClients(), glooDeployment, 1)

			By("force an update of the service endpoints")
			var initialEndpointIPs, newEndpointIPs []string
			initialEndpointIPs = endpointIPsForKubeService(resourceClientset.KubeClients(), appService)

			scaleDeploymentTo(resourceClientset.KubeClients(), appDeployment, 0)
			scaleDeploymentTo(resourceClientset.KubeClients(), appDeployment, 1)

			Eventually(func() []string {
				newEndpointIPs = endpointIPsForKubeService(resourceClientset.KubeClients(), appService)
				return newEndpointIPs
			}, 20*time.Second, 1*time.Second).Should(And(
				HaveLen(len(initialEndpointIPs)),
				Not(BeEquivalentTo(initialEndpointIPs)),
			))

			By("verify that the new endpoints have been propagated to Envoy")
			gatewayProxyPodName := findPodNamesByLabel(ctx, testHelper.InstallNamespace, "gloo=gateway-proxy")[0]
			envoyClustersPath := "http://localhost:19000/clusters" // TODO - this should live in envoy test service
			Eventually(func() bool {
				clusters := testutils.CurlWithEphemeralPodStable(ctx, io.Discard, "", testHelper.InstallNamespace, gatewayProxyPodName, envoyClustersPath)

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
						Image: kube2e.GetHttpEchoImage(),
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

func scaleDeploymentTo(kubeClient kubernetes.Interface, deploymentToScale *appsv1.Deployment, replicas int32) {
	// Do this in an Eventually block, as the update sometimes fails due to concurrent modification
	scaleCtx := context.Background()
	deploymentNamespace := deploymentToScale.Namespace
	EventuallyWithOffset(1, func() error {
		// Get deployment
		deployment, err := kubeClient.AppsV1().Deployments(deploymentNamespace).Get(scaleCtx, deploymentToScale.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Scale it
		deployment.Spec.Replicas = pointerToInt32(replicas)
		_, err = kubeClient.AppsV1().Deployments(deploymentNamespace).Update(scaleCtx, deployment, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(BeNil())

	// Wait for expected running pod number
	EventuallyWithOffset(1, func() error {
		listOpts := metav1.ListOptions{LabelSelector: labels.SelectorFromSet(deploymentToScale.Spec.Selector.MatchLabels).String()}
		pods, err := kubeClient.CoreV1().Pods(deploymentNamespace).List(scaleCtx, listOpts)
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
