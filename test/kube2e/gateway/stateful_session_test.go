package gateway_test

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

var _ = Describe("Stateful Session Tests", func() {
	var (
		appName       = "session-affinity"
		appDeployment *appsv1.Deployment
		appService    *corev1.Service
		appVs         *gatewayv1.VirtualService

		err error

		glooResources *gloosnapshot.ApiSnapshot
	)

	BeforeEach(func() {
		appDeployment, appService, err = createSessionAffinityDeploymentAndService(resourceClientset.KubeClients(), testHelper.InstallNamespace, appName)
		Expect(err).NotTo(HaveOccurred())

		appVs = helpers.NewVirtualServiceBuilder().
			WithName(appName).
			WithNamespace(testHelper.InstallNamespace).
			WithLabel(kube2e.UniqueTestResourceLabel, uuid.New().String()).
			WithDomain("app").
			WithRouteExactMatcher("count", "/count").
			WithRouteActionToSingleDestination("count",
				&gloov1.Destination{
					DestinationType: &gloov1.Destination_Kube{
						Kube: &gloov1.KubernetesServiceDestination{
							Ref: &core.ResourceRef{
								Namespace: testHelper.InstallNamespace,
								Name:      appService.Name,
							},
							Port: 8080,
						},
					},
				}).
			Build()

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

	FIt("should route to the same pod for the same session", func() {
		numRequests := 100

		By("Ensure we can route to the service")
		// Wait until we can get a response
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "http",
			Path:              "/count",
			Method:            "GET",
			Host:              "app",
			Service:           gatewayProxy,
			Port:              gatewayPort,
			ConnectionTimeout: 1,
			WithoutStats:      true,
			LogResponses:      true,
			Cookie:            "cookie.txt",
			CookieJar:         "cookie.txt",
			Verbose:           true,
		}, &matchers.HttpResponse{StatusCode: http.StatusOK, Body: "1"}, 1, 60*time.Second, 1*time.Second)

		// Once responses are coming, they should keep incrementing
		// Since we are usign a round-robin algorithm, and have one successful curl, we should hit the
		// other server and get respsonses from 2 ... numRequests
		for i := 2; i <= numRequests; i++ {
			fmt.Printf("*****\n	Curling for %d\n", i)
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/count",
				Method:            "GET",
				Host:              "app",
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
				LogResponses:      true,
				Cookie:            "cookie.txt",
				CookieJar:         "cookie.txt",
				Verbose:           true,
			}, &matchers.HttpResponse{StatusCode: http.StatusOK, Body: strconv.Itoa(i)}, 1, 0*time.Second)
			fmt.Printf("Success for %d!!!\n", i)
		}
	})

})

func createSessionAffinityDeploymentAndService(kubeClient kubernetes.Interface, namespace, appName string) (
	*appsv1.Deployment, *corev1.Service, error,
) {
	deployment, err := kubeClient.AppsV1().Deployments(namespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   appName,
			Labels: map[string]string{"app": appName},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointerToInt32(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": appName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": appName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "session-affinity",
						Image: kube2e.GetStatefulSessionImage(),
						Args:  []string{fmt.Sprintf("-text=%s", expectedResponse(appName))},
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: 8080,
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
			// Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": appName},
			Ports: []corev1.ServicePort{{
				Name:     "http",
				Port:     8080,
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, err
	}
	return deployment, service, nil
}
