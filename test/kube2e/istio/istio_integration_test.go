package istio_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
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
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Gloo + Istio integration tests", func() {
	var (
		//ctx    context.Context
		//cancel context.CancelFunc
		upstreamRef       core.ResourceRef
		serviceRef        = core.ResourceRef{Name: AppServiceName, Namespace: AppServiceNamespace}
		virtualServiceRef = core.ResourceRef{Name: "httpbin-vs", Namespace: "gloo-system"}
	)

	BeforeEach(func() {
		//ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		//defer cancel()
	})

	// Sets up HTTPBin services
	setupHTTPBinServices := func(port int32, targetPort int) {
		service := corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceRef.Name,
				Namespace: serviceRef.Namespace,
				Labels:    map[string]string{"app": "httpbin", "service": "httpbin"},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "http",
						Port: port,
					},
				},
				Selector: map[string]string{"app": "httpbin"},
			},
		}
		// set a targetPort if needed
		if targetPort != -1 {
			service.Spec.Ports[0].TargetPort = intstr.FromInt(targetPort)
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
			Name:      fmt.Sprintf("%s-%s-%d", AppServiceNamespace, AppServiceName, port),
			Namespace: "gloo-system",
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
				Domains: []string{"httpbin.local"},
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

	Context("port settings", func() {
		AfterEach(func() {
			var err error
			err = resourceClientSet.VirtualServiceClient().Delete(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
				return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
			})

			err = resourceClientSet.ServiceClient().Delete(serviceRef.Namespace, serviceRef.Name, clients.DeleteOpts{})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				_, err := resourceClientSet.ServiceClient().Read(serviceRef.Namespace, serviceRef.Name, clients.ReadOpts{})
				// we should receive a DNE error, meaning it's now deleted
				return err != nil && skerrors.IsNotExist(err)
			}, "5s", "1s").Should(BeTrue())

			err = resourceClientSet.UpstreamClient().Delete(upstreamRef.Namespace, upstreamRef.Name, clients.DeleteOpts{})
			helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
				return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
			})
		})

		table.DescribeTable("should act as expected with varied ports", func(port int32, targetPort int, expected int) {
			setupHTTPBinServices(port, targetPort)

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/get",
				Method:            "GET",
				Host:              "httpbin.local",
				Service:           defaults.GatewayProxyName,
				Port:              80,
				ConnectionTimeout: 10,
				Verbose:           true,
				WithoutStats:      true,
			}, fmt.Sprintf("HTTP/1.1 %d", expected), 1, time.Minute*1)
		},
			table.Entry("with non-matching, yet valid, port and target (app) port", int32(8000), AppPort, http.StatusOK),
			table.Entry("with matching port and target port", int32(80), AppPort, http.StatusOK),
			table.Entry("without target port, and port matching pod's port", int32(AppPort), -1, http.StatusOK),
			table.Entry("without target port, and port not matching app's port", int32(8000), -1, http.StatusServiceUnavailable),
			table.Entry("pointing to the wrong target port", int32(8000), AppPort+1, http.StatusServiceUnavailable), // or maybe 404?
		)
	})
})
