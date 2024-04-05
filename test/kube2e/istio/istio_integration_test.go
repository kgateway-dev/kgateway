package istio_test

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/test/kube2e"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/go-utils/testutils/exec"
	kubeService "github.com/solo-io/solo-kit/api/external/kubernetes/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skerrors "github.com/solo-io/solo-kit/pkg/errors"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	kubernetesplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e/helper"
)

const (
	httpbinName = "httpbin"
	httpbinPort = 8000
)

var _ = Describe("Gloo + Istio integration tests", func() {
	var (
		gatewayRef = core.ResourceRef{Name: "http", Namespace: "gloo-system"}
	)

	Context("port settings", func() {
		var (
			serviceRef        = core.ResourceRef{Name: helper.TestServerName, Namespace: defaults.GlooSystem}
			virtualServiceRef = core.ResourceRef{Name: helper.TestServerName, Namespace: defaults.GlooSystem}
			httpRouteRef      = core.ResourceRef{Name: helper.TestServerName, Namespace: defaults.GlooSystem}
			upstreamRef       core.ResourceRef
		)

		AfterEach(func() {
			var err error
			if useGlooGateway {
				err = resourceClientSet.KubernetesGatewayClient().HTTPRoutes(httpRouteRef.Namespace).Delete(ctx, httpRouteRef.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					_, err := resourceClientSet.KubernetesGatewayClient().HTTPRoutes(httpRouteRef.Namespace).Get(ctx, httpRouteRef.Name, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							// Route is not found, indicating successful deletion
							return true
						}
						fmt.Printf("Error getting HTTPRoute: %v\n", err)
						return false
					}
					// Route still exists
					return false
				}).Should(BeTrue(), "HTTPRoute should be deleted")
			} else {
				err = resourceClientSet.VirtualServiceClient().Delete(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.DeleteOpts{
					IgnoreNotExist: true,
				})
				Expect(err).NotTo(HaveOccurred())
				helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
					return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
				})
			}

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
					Labels:    map[string]string{"gloo": helper.TestServerName},
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
					Selector: map[string]string{"gloo": helper.TestServerName},
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

			// Check Endpoint is created with correct port before creating VirtualService
			Eventually(func(g Gomega) {
				endpoint, err := resourceClientSet.KubeClients().CoreV1().Endpoints(serviceRef.Namespace).Get(ctx, serviceRef.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.Subsets).To(HaveLen(1))
				Expect(endpoint.Subsets[0].Ports).To(HaveLen(1))
				Expect(endpoint.Subsets[0].Ports[0].Port).To(Equal(tPort.IntVal))
			}, "5s", "1s").Should(Succeed())

			// the upstream should be created by discovery service
			upstreamRef = core.ResourceRef{
				Name:      kubernetesplugin.UpstreamName(defaults.GlooSystem, helper.TestServerName, port),
				Namespace: defaults.GlooSystem,
			}

			if useGlooGateway {
				httpRoute := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      httpRouteRef.Name,
						Namespace: httpRouteRef.Namespace,
					},
					Spec: gwv1.HTTPRouteSpec{
						Hostnames: []gwv1.Hostname{gwv1.Hostname(helper.TestServerName)},
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{
								Name:      apiv1.ObjectName(gatewayRef.Name),
								Namespace: ptrTo(apiv1.Namespace(gatewayRef.Namespace)),
							}},
						},
						Rules: []gwv1.HTTPRouteRule{
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptrTo(gwv1.PathMatchPathPrefix),
											Value: ptrTo("/"),
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: apiv1.ObjectName(helper.TestServerName),
												Port: ptrTo(apiv1.PortNumber(port)),
											},
										},
									},
								},
							},
						},
					},
				}

				_, err = resourceClientSet.KubernetesGatewayClient().HTTPRoutes(httpRouteRef.Namespace).Create(ctx, httpRoute, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					route, err := resourceClientSet.KubernetesGatewayClient().HTTPRoutes(httpRouteRef.Namespace).Get(ctx, httpRouteRef.Name, metav1.GetOptions{})
					if err != nil {
						fmt.Printf("Error getting HTTPRoute: %v\n", err)
						return false
					}
					return route != nil
				}).Should(BeTrue(), "HttpRoute should be created")
			} else {
				// TODO(npolshak): Need to fix status on resource. This is a temporary deflake for https://github.com/solo-io/gloo/issues/8554.
				helpers.EventuallyResourceStatusMatches(func() (resources.InputResource, error) {
					return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
				},
					Or(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Accepted),
						}),
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Pending),
						}),
					),
				)

				virtualService := &v1.VirtualService{
					Metadata: &core.Metadata{
						Name:      virtualServiceRef.Name,
						Namespace: virtualServiceRef.Namespace,
					},
					VirtualHost: &v1.VirtualHost{
						Domains: []string{helper.TestServerName},
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
				// TODO(npolshak): Need to fix status on resource. This is a temporary deflake for https://github.com/solo-io/gloo/issues/8554.
				helpers.EventuallyResourceStatusMatches(func() (resources.InputResource, error) {
					return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
				},
					Or(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Accepted),
						}),
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Pending),
						}),
					),
				)
			}
		}

		DescribeTable("should act as expected with varied ports", func(port int32, targetPort int, expected int) {
			setupServices(port, targetPort)

			var gatewayProxyName string
			if useGlooGateway {
				gatewayProxyName = glooGatewayProxy
			} else {
				gatewayProxyName = gatewayProxy
			}

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              helper.TestServerName,
				Service:           gatewayProxyName,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
				Verbose:           false,
				WithoutStats:      true,
				ReturnHeaders:     true,
			}, &testmatchers.HttpResponse{
				Body:       ContainSubstring(fmt.Sprintf("HTTP/1.1 %d", expected)),
				StatusCode: expected,
			}, 1, time.Minute*1)
		},
			Entry("with non-matching, yet valid, port and target (app) port", int32(helper.TestServerPort+1), helper.TestServerPort, http.StatusOK),
			Entry("with matching port and target port", int32(helper.TestServerPort), helper.TestServerPort, http.StatusOK),
			Entry("without target port, and port matching pod's port", int32(helper.TestServerPort), -1, http.StatusOK),
			Entry("without target port, and port not matching app's port", int32(helper.TestServerPort+1), -1, http.StatusServiceUnavailable),
			Entry("pointing to the wrong target port", int32(8000), helper.TestServerPort+1, http.StatusServiceUnavailable),
		)
	})

	Context("headless services", func() {
		var (
			headlessServiceRef        = core.ResourceRef{Name: "headless-svc", Namespace: "gloo-system"}
			headlessVirtualServiceRef = core.ResourceRef{Name: "headless-vs", Namespace: "gloo-system"}
			headlessHttpRouteRef      = core.ResourceRef{Name: "headless-httproute", Namespace: "gloo-system"}
			upstreamRef               core.ResourceRef
		)

		BeforeEach(func() {

			// create a headless service routed to testserver
			service := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      headlessServiceRef.Name,
					Namespace: headlessServiceRef.Namespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "None",
					Ports: []corev1.ServicePort{
						{
							Port:     helper.TestServerPort,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Selector: map[string]string{"gloo": "testserver"},
				},
			}
			var err error
			_, err = resourceClientSet.ServiceClient().Write(
				&kubernetes.Service{Service: kubeService.Service{Service: service}},
				clients.WriteOpts{},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				_, err := resourceClientSet.ServiceClient().Read(headlessServiceRef.Namespace, headlessServiceRef.Name, clients.ReadOpts{})
				return err
			}, "5s", "1s").Should(BeNil())

			// the upstream should be created by discovery service
			upstreamRef = core.ResourceRef{
				Name:      kubernetesplugin.UpstreamName(headlessServiceRef.Namespace, headlessServiceRef.Name, helper.TestServerPort),
				Namespace: defaults.GlooSystem,
			}

			if useGlooGateway {
				// create HTTPRoute routing to the headless service's upstream
				httpRoute := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      headlessHttpRouteRef.Name,
						Namespace: headlessHttpRouteRef.Namespace,
					},
					Spec: gwv1.HTTPRouteSpec{
						Hostnames: []gwv1.Hostname{gwv1.Hostname("headless.local")},
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{
								Name:      apiv1.ObjectName(gatewayRef.Name),
								Namespace: ptrTo(apiv1.Namespace(gatewayRef.Namespace)),
							}},
						},
						Rules: []gwv1.HTTPRouteRule{
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptrTo(gwv1.PathMatchPathPrefix),
											Value: ptrTo("/"),
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: apiv1.ObjectName(headlessServiceRef.Name),
												Port: ptrTo(apiv1.PortNumber(helper.TestServerPort)),
											},
										},
									},
								},
							},
						},
					},
				}

				_, err = resourceClientSet.KubernetesGatewayClient().HTTPRoutes(headlessHttpRouteRef.Namespace).Create(ctx, httpRoute, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					route, err := resourceClientSet.KubernetesGatewayClient().HTTPRoutes(headlessHttpRouteRef.Namespace).Get(ctx, headlessHttpRouteRef.Name, metav1.GetOptions{})
					if err != nil {
						fmt.Printf("Error getting HTTPRoute: %v\n", err)
						return false
					}
					return route != nil
				}).Should(BeTrue(), "HttpRoute should be created")
			} else {
				// check upstream is created
				// TODO(npolshak): Need to fix status on resource. This is a temporary deflake for https://github.com/solo-io/gloo/issues/8554.
				helpers.EventuallyResourceStatusMatches(func() (resources.InputResource, error) {
					return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
				},
					Or(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Accepted),
						}),
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Pending),
						}),
					),
				)

				// create virtual service routing to the headless service's upstream
				virtualService := &v1.VirtualService{
					Metadata: &core.Metadata{
						Name:      headlessVirtualServiceRef.Name,
						Namespace: headlessVirtualServiceRef.Namespace,
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
				// TODO(npolshak): Need to fix status on resource. This is a temporary deflake for https://github.com/solo-io/gloo/issues/8554.
				helpers.EventuallyResourceStatusMatches(func() (resources.InputResource, error) {
					return resourceClientSet.VirtualServiceClient().Read(headlessVirtualServiceRef.Namespace, headlessVirtualServiceRef.Name, clients.ReadOpts{})
				},
					Or(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Accepted),
						}),
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Pending),
						}),
					),
				)
			}
		})

		AfterEach(func() {
			var err error
			if useGlooGateway {
				err = resourceClientSet.KubernetesGatewayClient().HTTPRoutes(headlessHttpRouteRef.Namespace).Delete(ctx, headlessHttpRouteRef.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					_, err := resourceClientSet.KubernetesGatewayClient().HTTPRoutes(headlessHttpRouteRef.Namespace).Get(ctx, headlessHttpRouteRef.Name, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							// Route is not found, indicating successful deletion
							return true
						}
						fmt.Printf("Error getting HTTPRoute: %v\n", err)
						return false
					}
					// Route still exists
					return false
				}).Should(BeTrue(), "HTTPRoute should be deleted")
			} else {
				err = resourceClientSet.VirtualServiceClient().Delete(headlessVirtualServiceRef.Namespace, headlessVirtualServiceRef.Name, clients.DeleteOpts{
					IgnoreNotExist: true,
				})
				Expect(err).NotTo(HaveOccurred())
				helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
					return resourceClientSet.VirtualServiceClient().Read(headlessVirtualServiceRef.Namespace, headlessVirtualServiceRef.Name, clients.ReadOpts{})
				})
			}

			err = resourceClientSet.ServiceClient().Delete(headlessServiceRef.Namespace, headlessServiceRef.Name, clients.DeleteOpts{
				IgnoreNotExist: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				_, err := resourceClientSet.ServiceClient().Read(headlessServiceRef.Namespace, headlessServiceRef.Name, clients.ReadOpts{})
				// we should receive a DNE error, meaning it's now deleted
				return err != nil && skerrors.IsNotExist(err)
			}, "5s", "1s").Should(BeTrue())

			if !useGlooGateway {
				err = resourceClientSet.UpstreamClient().Delete(upstreamRef.Namespace, upstreamRef.Name, clients.DeleteOpts{
					IgnoreNotExist: true,
				})
				helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
					return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
				})
			}
		})

		It("routes to headless services", func() {
			var gatewayProxyName string
			if useGlooGateway {
				gatewayProxyName = glooGatewayProxy
			} else {
				gatewayProxyName = gatewayProxy
			}

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              "headless.local",
				Service:           gatewayProxyName,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
				Verbose:           false,
				WithoutStats:      true,
				ReturnHeaders:     true,
			}, &testmatchers.HttpResponse{
				Body:       ContainSubstring(fmt.Sprintf("HTTP/1.1 %d", http.StatusOK)),
				StatusCode: http.StatusOK,
			}, 1, time.Minute*1)
		})
	})

	Context("Istio mTLS", func() {
		httpbinVirtualServiceRef := core.ResourceRef{Name: httpbinName, Namespace: installNamespace}
		httpbinHttpRouteRef := core.ResourceRef{Name: httpbinName, Namespace: "httpbin-ns"}

		// the upstream should be created by discovery service
		httpbinUpstreamRef := core.ResourceRef{
			Name:      kubernetesplugin.UpstreamName(httpbinNamespace, httpbinName, httpbinPort),
			Namespace: installNamespace,
		}

		BeforeEach(func() {

			if useGlooGateway {
				// TODO(npolshak): Add HTTPRoute builder as part of e2e test framework
				// create HTTPRoute routing to the headless service's upstream
				httpRoute := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      httpbinHttpRouteRef.Name,
						Namespace: httpbinHttpRouteRef.Namespace,
					},
					Spec: gwv1.HTTPRouteSpec{
						Hostnames: []gwv1.Hostname{gwv1.Hostname(httpbinName)},
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{
								Name:      apiv1.ObjectName(gatewayRef.Name),
								Namespace: ptrTo(apiv1.Namespace(gatewayRef.Namespace)),
							}},
						},
						Rules: []gwv1.HTTPRouteRule{
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptrTo(gwv1.PathMatchPathPrefix),
											Value: ptrTo("/"),
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: apiv1.ObjectName(httpbinName),
												Port: ptrTo(apiv1.PortNumber(httpbinPort)),
											},
										},
									},
								},
							},
						},
					},
				}

				_, err := resourceClientSet.KubernetesGatewayClient().HTTPRoutes(httpbinHttpRouteRef.Namespace).Create(ctx, httpRoute, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					route, err := resourceClientSet.KubernetesGatewayClient().HTTPRoutes(httpbinHttpRouteRef.Namespace).Get(ctx, httpbinHttpRouteRef.Name, metav1.GetOptions{})
					if err != nil {
						fmt.Printf("Error getting HTTPRoute: %v\n", err)
						return false
					}
					return route != nil
				}).Should(BeTrue(), "HttpRoute should be created")
			} else {
				// check upstream is created
				// TODO(npolshak): Need to fix status on resource. This is a temporary deflake for https://github.com/solo-io/gloo/issues/8554.
				helpers.EventuallyResourceStatusMatches(func() (resources.InputResource, error) {
					return resourceClientSet.UpstreamClient().Read(httpbinUpstreamRef.Namespace, httpbinUpstreamRef.Name, clients.ReadOpts{})
				},
					Or(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Accepted),
						}),
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Pending),
						}),
					),
				)

				route := helpers.NewRouteBuilder().
					WithRouteActionToUpstreamRef(&httpbinUpstreamRef).
					WithMatcher(&matchers.Matcher{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/",
						},
					}).
					Build()

				vs := helpers.NewVirtualServiceBuilder().
					WithName(httpbinVirtualServiceRef.Name).
					WithNamespace(httpbinVirtualServiceRef.Namespace).
					WithDomain(httpbinName).
					WithRoute("default-route", route).
					Build()
				_, err := resourceClientSet.VirtualServiceClient().Write(vs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())
				// TODO(npolshak): Need to fix status on resource. This is a temporary deflake for https://github.com/solo-io/gloo/issues/8554.
				helpers.EventuallyResourceStatusMatches(func() (resources.InputResource, error) {
					return resourceClientSet.VirtualServiceClient().Read(httpbinVirtualServiceRef.Namespace, httpbinVirtualServiceRef.Name, clients.ReadOpts{})
				},
					Or(
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Accepted),
						}),
						gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal(core.Status_Pending),
						}),
					),
				)
			}
		})

		AfterEach(func() {
			var err error

			if useGlooGateway {
				err = resourceClientSet.KubernetesGatewayClient().HTTPRoutes(httpbinHttpRouteRef.Namespace).Delete(ctx, httpbinHttpRouteRef.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					_, err := resourceClientSet.KubernetesGatewayClient().HTTPRoutes(httpbinHttpRouteRef.Namespace).Get(ctx, httpbinHttpRouteRef.Name, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							// Route is not found, indicating successful deletion
							return true
						}
						fmt.Printf("Error getting HTTPRoute: %v\n", err)
						return false
					}
					// Route still exists
					return false
				}).Should(BeTrue(), "HTTPRoute should be deleted")
			} else {
				err = resourceClientSet.VirtualServiceClient().Delete(httpbinVirtualServiceRef.Namespace, httpbinVirtualServiceRef.Name, clients.DeleteOpts{
					IgnoreNotExist: true,
				})
				Expect(err).NotTo(HaveOccurred())
				helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
					return resourceClientSet.VirtualServiceClient().Read(httpbinVirtualServiceRef.Namespace, httpbinVirtualServiceRef.Name, clients.ReadOpts{})
				})

				err = resourceClientSet.UpstreamClient().Delete(httpbinUpstreamRef.Namespace, httpbinUpstreamRef.Name, clients.DeleteOpts{
					IgnoreNotExist: true,
				})
				helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
					return resourceClientSet.UpstreamClient().Read(httpbinUpstreamRef.Namespace, httpbinUpstreamRef.Name, clients.ReadOpts{})
				})
			}
		})

		Context("permissive peer auth", func() {
			BeforeEach(func() {
				err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", filepath.Join(cwd, "artifacts", "peerauth_permissive.yaml"))
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-n", "istio-system", "peerauthentication", "test")
				Expect(err).NotTo(HaveOccurred())
			})

			When("mtls is not enabled for the upstream", func() {

				It("should be able to complete the request without mTLS header", func() {
					var gatewayProxyName string
					if useGlooGateway {
						gatewayProxyName = glooGatewayProxy
					} else {
						gatewayProxyName = gatewayProxy
					}

					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/",
						Method:            "GET",
						Host:              httpbinName,
						Service:           gatewayProxyName,
						Port:              gatewayPort,
						ConnectionTimeout: 10,
						Verbose:           false,
						WithoutStats:      true,
						ReturnHeaders:     false,
					}, &testmatchers.HttpResponse{
						Body:       ContainSubstring("200"),
						StatusCode: http.StatusOK,
					}, 1, time.Minute)
				})
			})

			When("mtls is enabled for the upstream", func() {
				BeforeEach(func() {
					// auto mtls is used instead for GlooGateway and discovery is turned off
					if !useGlooGateway {
						// Other tests create/delete upstream, we need to wait for the upstream to be created
						EventuallyWithOffset(1, func(g Gomega) {
							err := testutils.Glooctl(fmt.Sprintf("istio enable-mtls --upstream %s", httpbinUpstreamRef.Name))
							Expect(err).NotTo(HaveOccurred())
						}, 30*time.Second).ShouldNot(HaveOccurred())
					}
				})

				AfterEach(func() {
					if !useGlooGateway {
						// It seems to sometimes take multiple calls before the disable command is registered
						EventuallyWithOffset(1, func(g Gomega) {
							err := testutils.Glooctl(fmt.Sprintf("istio disable-mtls --upstream %s", httpbinUpstreamRef.Name))
							g.Expect(err).NotTo(HaveOccurred())
							us, err := resourceClientSet.UpstreamClient().Read(httpbinUpstreamRef.Namespace, httpbinUpstreamRef.Name, clients.ReadOpts{})
							g.Expect(err).NotTo(HaveOccurred())
							g.Expect(us.SslConfig).To(BeNil())
						}, 30*time.Second).ShouldNot(HaveOccurred())
					}
				})

				It("should make a request with the expected cert header", func() {
					var gatewayProxyName string
					if useGlooGateway {
						gatewayProxyName = glooGatewayProxy
					} else {
						gatewayProxyName = gatewayProxy
					}

					// the /headers endpoint will respond with the headers the request to the client contains
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/headers",
						Method:            "GET",
						Host:              httpbinName,
						Service:           gatewayProxyName,
						Port:              gatewayPort,
						ConnectionTimeout: 10,
						Verbose:           false,
						WithoutStats:      true,
						ReturnHeaders:     false,
					}, "\"X-Forwarded-Client-Cert\"", 1, time.Minute)
				})
			})
		})

		When("mtls disabled for the upstream", func() {
			BeforeEach(func() {
				useGlooGateway := useGlooGateway
				if useGlooGateway {
					Skip("Gloo Gateway does not support Upstream overwrites for mTLS")
				}

				// update upstream to disable auto mtls
				var httpbinUpstream *gloov1.Upstream
				var err error
				// wait for upstream to be created by discovery before editing
				Eventually(func() error {
					httpbinUpstream, err = resourceClientSet.UpstreamClient().Read(httpbinUpstreamRef.Namespace, httpbinUpstreamRef.Name, clients.ReadOpts{})
					return err
				}, "30s", "1s").Should(BeNil())
				httpbinUpstream.DisableIstioAutoMtls = &wrapperspb.BoolValue{Value: true}
				// wait for upstream to be updated
				Eventually(func() error {
					_, err = resourceClientSet.UpstreamClient().Write(httpbinUpstream, clients.WriteOpts{OverwriteExisting: true})
					return err
				}, "30s", "1s").Should(BeNil())

				// apply peerauth to only allow requests without mTLS
				err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", filepath.Join(cwd, "artifacts", "peerauth_disable.yaml"))
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-n", "istio-system", "peerauthentication", "test")
				Expect(err).NotTo(HaveOccurred())

				// revert upstream to enable auto mtls
				var httpbinUpstream *gloov1.Upstream
				Eventually(func() error {
					httpbinUpstream, err = resourceClientSet.UpstreamClient().Read(httpbinUpstreamRef.Namespace, httpbinUpstreamRef.Name, clients.ReadOpts{})
					return err
				}, "30s", "1s").Should(BeNil())
				httpbinUpstream.DisableIstioAutoMtls = nil
				// wait for upstream to be updated
				Eventually(func() error {
					_, err = resourceClientSet.UpstreamClient().Write(httpbinUpstream, clients.WriteOpts{OverwriteExisting: true})
					return err
				}, "30s", "1s").Should(BeNil())
			})

			It("should make a request with the expected cert header", func() {
				var gatewayProxyName string
				if useGlooGateway {
					gatewayProxyName = glooGatewayProxy
				} else {
					gatewayProxyName = gatewayProxy
				}

				// Should still be able to reach endpoint without mTLS
				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              httpbinName,
					Service:           gatewayProxyName,
					Port:              gatewayPort,
					ConnectionTimeout: 10,
					Verbose:           false,
					WithoutStats:      true,
					ReturnHeaders:     false,
				}, "200", 1, time.Minute)
			})
		})

		Context("strict peer auth", func() {
			BeforeEach(func() {
				err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", filepath.Join(cwd, "artifacts", "peerauth_strict.yaml"))
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-n", "istio-system", "peerauthentication", "test")
				Expect(err).NotTo(HaveOccurred())
			})

			When("mtls is not enabled for the upstream", func() {

				BeforeEach(func() {
					// Disable auto mtls for Gloo Gateway to show strict peer auth is enforced if not used
					if useGlooGateway || useAutoMtls {
						kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
							Expect(settings.GetGateway().GetCompressedProxySpec()).NotTo(BeNil())
							settings.Gloo.IstioOptions.EnableAutoMtls = &wrapperspb.BoolValue{Value: false}
						}, testHelper.InstallNamespace)
					}
				})

				AfterEach(func() {
					// Re-enable auto mtls for Gloo Gateway since other tests depend on it
					if useGlooGateway || useAutoMtls {
						kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
							Expect(settings.GetGateway().GetCompressedProxySpec()).NotTo(BeNil())
							settings.Gloo.IstioOptions.EnableAutoMtls = &wrapperspb.BoolValue{Value: true}
						}, testHelper.InstallNamespace)
					}
				})

				It("should not be able to complete the request", func() {
					var gatewayProxyName string
					if useGlooGateway {
						gatewayProxyName = glooGatewayProxy
					} else {
						gatewayProxyName = gatewayProxy
					}

					// the /headers endpoint will respond with the headers the request to the client contains
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/headers",
						Method:            "GET",
						Host:              httpbinName,
						Service:           gatewayProxyName,
						Port:              gatewayPort,
						ConnectionTimeout: 10,
						Verbose:           false,
						WithoutStats:      true,
						ReturnHeaders:     false,
					}, &testmatchers.HttpResponse{
						StatusCode: 503,
						Body:       ContainSubstring("upstream connect error or disconnect/reset before headers. reset reason: connection termination"),
					}, 1, time.Minute*1)
				})
			})

			When("mtls is enabled for the upstream", func() {
				BeforeEach(func() {
					// Gloo Gateway relies on auto mtls and has discovery disabled so no upstreams are created
					if !useGlooGateway {
						// Other tests create/delete upstream, we need to wait for the upstream to be created
						EventuallyWithOffset(1, func(g Gomega) {
							err := testutils.Glooctl(fmt.Sprintf("istio enable-mtls --upstream %s", httpbinUpstreamRef.Name))
							Expect(err).NotTo(HaveOccurred())
						}, 30*time.Second).ShouldNot(HaveOccurred())
					}
				})

				AfterEach(func() {
					if !useGlooGateway {
						// It seems to sometimes take multiple calls before the disable command is registered
						EventuallyWithOffset(1, func(g Gomega) {
							err := testutils.Glooctl(fmt.Sprintf("istio disable-mtls --upstream %s", httpbinUpstreamRef.Name))
							g.Expect(err).NotTo(HaveOccurred())
							us, err := resourceClientSet.UpstreamClient().Read(httpbinUpstreamRef.Namespace, httpbinUpstreamRef.Name, clients.ReadOpts{})
							g.Expect(err).NotTo(HaveOccurred())
							g.Expect(us.SslConfig).To(BeNil())
						}, 30*time.Second).ShouldNot(HaveOccurred())
					}
				})

				It("should make a request with the expected cert header", func() {
					var gatewayProxyName string
					if useGlooGateway {
						gatewayProxyName = glooGatewayProxy

					} else {
						gatewayProxyName = gatewayProxy
					}
					// the /headers endpoint will respond with the headers the request to the client contains
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/headers",
						Method:            "GET",
						Host:              httpbinName,
						Service:           gatewayProxyName,
						Port:              gatewayPort,
						ConnectionTimeout: 10,
						Verbose:           false,
						WithoutStats:      true,
						ReturnHeaders:     false,
					}, "\"X-Forwarded-Client-Cert\"", 1, time.Minute*1)
				})
			})
		})
	})
})

// gateway apis uses this to build test examples: https://github.com/kubernetes-sigs/gateway-api/blob/main/pkg/test/cel/main_test.go#L57
func ptrTo[T any](a T) *T {
	return &a
}
