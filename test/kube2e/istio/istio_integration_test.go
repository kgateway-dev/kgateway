package istio_test

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/test/helpers"
	kubeService "github.com/solo-io/solo-kit/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Gloo + Istio integration tests", func() {
	var (
	//ctx    context.Context
	//cancel context.CancelFunc
	)

	BeforeEach(func() {
		//ctx, cancel = context.WithCancel(context.Background())

	})

	AfterEach(func() {
		//defer cancel()
	})

	// Sets up HTTPBin services
	//setupHTTPBinServices := func(port int32, targetPort int) {
	//service := core_v1.Service{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name:   "httpbin",
	//		Labels: map[string]string{"app": "httpbin", "service": "httpbin"},
	//	},
	//	Spec: core_v1.ServiceSpec{
	//		Ports: []core_v1.ServicePort{
	//			{
	//				Name:       "http",
	//				Port:       port,
	//				TargetPort: intstr.FromInt(targetPort),
	//			},
	//		},
	//		Selector: map[string]string{"app": "httpbin"},
	//	},
	//}
	//_, err := serviceClient.Write(
	//	&kubernetes.Service{Service: kubeService.Service{Service: service}},
	//	clients.WriteOpts{},
	//)
	//Expect(err).NotTo(HaveOccurred())
	//Eventually(func() error {
	//	services, err := serviceClient.List("default", clients.ListOpts{})
	//	if err != nil {
	//		return err
	//	}
	//
	//	if _, err = services.Find("default", "httpbin"); err != nil {
	//		return fmt.Errorf("expected service httpbin to exists")
	//	}
	//	return nil
	//}, "5s", "1s").Should(BeNil())
	//// todo - wait for upstream to exist, to use in VS?
	//
	//virtualService := &v1.VirtualService{
	//	Metadata: &core.Metadata{
	//		Name:      "httpbin-vs",
	//		Namespace: "gloo-system",
	//	},
	//	VirtualHost: &v1.VirtualHost{
	//		Domains: []string{"httpbin.local"},
	//		Routes: []*v1.Route{{
	//			Action: &v1.Route_RouteAction{
	//				RouteAction: &gloov1.RouteAction{
	//					Destination: &gloov1.RouteAction_Single{
	//						Single: &gloov1.Destination{
	//							DestinationType: &gloov1.Destination_Upstream{
	//								Upstream: &core.ResourceRef{
	//									Name: fmt.Sprintf("default-httpbin-%d", port),
	//								},
	//							},
	//						},
	//					},
	//				},
	//			},
	//			Matchers: []*matchers.Matcher{
	//				{
	//					PathSpecifier: &matchers.Matcher_Prefix{
	//						Prefix: "/",
	//					},
	//				},
	//			},
	//		}},
	//	},
	//}
	//virtualServiceClient.Write(virtualService, clients.WriteOpts{})
	//Eventually(func() error {
	//	virtualServices, err := virtualServiceClient.List("gloo-system", clients.ListOpts{})
	//	if err != nil {
	//		return err
	//	}
	//
	//	if _, err = virtualServices.Find("gloo-system", "httpbin-vs"); err != nil {
	//		return fmt.Errorf("expected virtual service httpbin-vs to exists")
	//	}
	//	return nil
	//}, "5s", "1s").Should(BeNil())
	//}

	//Context("port settings", func() {
	//	table.DescribeTable("should act as expected with varied ports", func(port int32, targetPort int, expected int) {
	//		setupHTTPBinServices(port, targetPort)
	//
	//		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
	//			Protocol:          "http",
	//			Path:              "/get",
	//			Method:            "GET",
	//			Host:              "httpbin.local",
	//			Service:           defaults.GatewayProxyName,
	//			Port:              8080,
	//			ConnectionTimeout: 10,
	//		}, fmt.Sprintf("%d", expected), 1, time.Minute*1)
	//	},
	//		table.Entry("with matching port and target port", 80, AppPort, http.StatusOK),
	//		table.Entry("without target port", 8000, 0, http.StatusOK),
	//		table.Entry("with non-matching, yet valid, port and target port", 8000, AppPort, http.StatusOK),
	//		table.Entry("pointing to the wrong target port", 8000, AppPort+1, http.StatusServiceUnavailable), // or maybe 404?
	//	)
	//})

	FContext("test service + vs + upstream setup", func() {
		It("something", func() {
			service := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      AppServiceName,
					Namespace: AppServiceNamespace,
					Labels:    map[string]string{"app": "httpbin", "service": "httpbin"},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,                   // todo configurable for tests
							TargetPort: intstr.FromInt(8000), // todo configurable for tests
						},
					},
					Selector: map[string]string{"app": "httpbin"},
				},
			}
			var err error
			_, err = resourceClientSet.ServiceClient().Write(
				&kubernetes.Service{Service: kubeService.Service{Service: service}},
				clients.WriteOpts{},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				_, err := resourceClientSet.ServiceClient().Read(AppServiceNamespace, AppServiceName, clients.ReadOpts{})
				return err
			}, "5s", "1s").Should(BeNil())
			// the upstream should be created by discovery service
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				upstreamName := fmt.Sprintf("%s-%s-%d", AppServiceNamespace, AppServiceName, 80)
				return resourceClientSet.UpstreamClient().Read("gloo-system", upstreamName, clients.ReadOpts{})
			})
			// it works so far... (need to do a beforeEach/afterEach to delete the created stuff automatically)

			virtualService := &v1.VirtualService{
				Metadata: &core.Metadata{
					Name:      "httpbin-vs",
					Namespace: "gloo-system",
				},
				VirtualHost: &v1.VirtualHost{
					Domains: []string{"httpbin.local"},
					Routes: []*v1.Route{{
						Action: &v1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name: fmt.Sprintf("%s-%s-%d", AppServiceNamespace, AppServiceName, 80),
											},
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
				return resourceClientSet.VirtualServiceClient().Read("gloo-system", "httpbin-vs", clients.ReadOpts{})
			})
		})
	})
})
