package istio_test

import (
	"fmt"
	"net/http"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	kubernetesplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"

	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/k8s-utils/testutils/helper"
	kubeService "github.com/solo-io/solo-kit/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skerrors "github.com/solo-io/solo-kit/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Gloo + Istio integration tests", func() {
	var (
		upstreamRef       core.ResourceRef
		serviceRef        = core.ResourceRef{Name: helper.TestrunnerName, Namespace: "gloo-system"}
		virtualServiceRef = core.ResourceRef{Name: helper.TestrunnerName, Namespace: "gloo-system"}
	)

	AfterEach(func() {
		var err error
		err = resourceClientSet.VirtualServiceClient().Delete(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.DeleteOpts{
			IgnoreNotExist: true,
		})
		Expect(err).NotTo(HaveOccurred())
		helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
			return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
		})

		err = resourceClientSet.ServiceClient().Delete(serviceRef.Namespace, serviceRef.Name, clients.DeleteOpts{
			IgnoreNotExist: true,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() bool {
			_, err := resourceClientSet.ServiceClient().Read(serviceRef.Namespace, serviceRef.Name, clients.ReadOpts{})
			// we should receive a DNE error, meaning it's now deleted
			return err != nil && skerrors.IsNotExist(err)
		}, "5s", "1s").Should(BeTrue())

		err = resourceClientSet.UpstreamClient().Delete(upstreamRef.Namespace, upstreamRef.Name, clients.DeleteOpts{
			IgnoreNotExist: true,
		})
		helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
			return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
		})
	})

	Context("port settings", func() {
		BeforeEach(func() {
			serviceRef = core.ResourceRef{Name: helper.TestrunnerName, Namespace: defaults.GlooSystem}
			virtualServiceRef = core.ResourceRef{Name: helper.TestrunnerName, Namespace: defaults.GlooSystem}
		})

		// Sets up services
		setupServices := func(port int32, targetPort int) {
			// A Service's TargetPort defaults to the Port if not set
			tPort := intstr.FromInt(int(port))
			if targetPort != -1 {
				tPort = intstr.FromInt(targetPort)
			}
			service := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceRef.Name,
					Namespace: serviceRef.Namespace,
					Labels:    map[string]string{"gloo": helper.TestrunnerName},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       port,
							TargetPort: tPort,
							Protocol:   corev1.ProtocolTCP,
						},
					},
					Selector: map[string]string{"gloo": helper.TestrunnerName},
				},
			}
			var err error
			_, err = resourceClientSet.ServiceClient().Write(
				&kubernetes.Service{Service: kubeService.Service{Service: service}},
				clients.WriteOpts{},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				_, err := resourceClientSet.ServiceClient().Read(serviceRef.Namespace, service.Name, clients.ReadOpts{})
				return err
			}, "5s", "1s").Should(BeNil())

			// the upstream should be created by discovery service
			upstreamRef = core.ResourceRef{
				Name:      kubernetesplugin.UpstreamName(defaults.GlooSystem, helper.TestrunnerName, port),
				Namespace: defaults.GlooSystem,
			}
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
			})

			virtualService := &v1.VirtualService{
				Metadata: &core.Metadata{
					Name:      virtualServiceRef.Name,
					Namespace: virtualServiceRef.Namespace,
				},
				VirtualHost: &v1.VirtualHost{
					Domains: []string{helper.TestrunnerName},
					Routes: []*v1.Route{{
						Action: &v1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &upstreamRef,
										},
									},
								},
							},
						},
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/",
								},
							},
						},
					}},
				},
			}
			_, err = resourceClientSet.VirtualServiceClient().Write(virtualService, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
			})
		}

		DescribeTable("should act as expected with varied ports", func(port int32, targetPort int, expected int) {
			setupServices(port, targetPort)

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              helper.TestrunnerName,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
				Verbose:           false,
				WithoutStats:      true,
				ReturnHeaders:     true,
			}, fmt.Sprintf("HTTP/1.1 %d", expected), 1, time.Minute*1)
		},
			Entry("with non-matching, yet valid, port and target (app) port", int32(helper.TestRunnerPort+1), helper.TestRunnerPort, http.StatusOK),
			Entry("with matching port and target port", int32(helper.TestRunnerPort), helper.TestRunnerPort, http.StatusOK),
			Entry("without target port, and port matching pod's port", int32(helper.TestRunnerPort), -1, http.StatusOK),
			Entry("without target port, and port not matching app's port", int32(helper.TestRunnerPort+1), -1, http.StatusServiceUnavailable),
			Entry("pointing to the wrong target port", int32(8000), helper.TestRunnerPort+1, http.StatusServiceUnavailable), // or maybe 404?
		)
	})

	Context("headless services", func() {
		BeforeEach(func() {
			serviceRef = core.ResourceRef{Name: "headless-svc", Namespace: "gloo-system"}
			virtualServiceRef = core.ResourceRef{Name: "headless-vs", Namespace: "gloo-system"}

			// create a headless service routed to testrunner
			service := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceRef.Name,
					Namespace: serviceRef.Namespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "None",
					Ports: []corev1.ServicePort{
						{
							Port:     helper.TestRunnerPort,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Selector: map[string]string{"gloo": "testrunner"},
				},
			}
			var err error
			_, err = resourceClientSet.ServiceClient().Write(
				&kubernetes.Service{Service: kubeService.Service{Service: service}},
				clients.WriteOpts{},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				_, err := resourceClientSet.ServiceClient().Read(serviceRef.Namespace, serviceRef.Name, clients.ReadOpts{})
				return err
			}, "5s", "1s").Should(BeNil())

			// the upstream should be created by discovery service
			upstreamRef = core.ResourceRef{
				Name:      kubernetesplugin.UpstreamName(serviceRef.Namespace, serviceRef.Name, helper.TestRunnerPort),
				Namespace: defaults.GlooSystem,
			}
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
			})

			// create virtual service routing to the headless service's upstream
			virtualService := &v1.VirtualService{
				Metadata: &core.Metadata{
					Name:      virtualServiceRef.Name,
					Namespace: virtualServiceRef.Namespace,
				},
				VirtualHost: &v1.VirtualHost{
					Domains: []string{"headless.local"},
					Routes: []*v1.Route{{
						Action: &v1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &upstreamRef,
										},
									},
								},
							},
						},
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/",
								},
							},
						},
					}},
				},
			}
			_, err = resourceClientSet.VirtualServiceClient().Write(virtualService, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
			})
		})

		It("routes to headless services", func() {
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              "headless.local",
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
				Verbose:           false,
				WithoutStats:      true,
				ReturnHeaders:     true,
			}, fmt.Sprintf("HTTP/1.1 %d", http.StatusOK), 1, time.Minute*1)
		})
	})
})
