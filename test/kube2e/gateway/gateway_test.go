package gateway_test

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/google/uuid"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/compress"

	gloo_matchers "github.com/solo-io/solo-kit/test/matchers"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	"google.golang.org/grpc"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	grpcv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	glootransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	defaults2 "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	kubernetesplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/linkerd"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Kube2e: gateway", func() {

	var (
		testServerDestination *gloov1.Destination
		testServerVs          *gatewayv1.VirtualService

		glooResources *gloosnapshot.ApiSnapshot
	)

	BeforeEach(func() {
		// Create a VirtualService routing directly to the testserver kubernetes service
		testServerDestination = &gloov1.Destination{
			DestinationType: &gloov1.Destination_Kube{
				Kube: &gloov1.KubernetesServiceDestination{
					Ref: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      helper.TestServerName,
					},
					Port: uint32(helper.TestServerPort),
				},
			},
		}
		testServerVs = helpers.NewVirtualServiceBuilder().
			WithName(helper.TestServerName).
			WithNamespace(testHelper.InstallNamespace).
			WithLabel(kube2e.UniqueTestResourceLabel, uuid.New().String()).
			WithDomain(helper.TestServerName).
			WithRoutePrefixMatcher(helper.TestServerName, "/").
			WithRouteActionToSingleDestination(helper.TestServerName, testServerDestination).
			Build()

		// The set of resources that these tests will generate
		glooResources = &gloosnapshot.ApiSnapshot{
			VirtualServices: gatewayv1.VirtualServiceList{
				// many tests route to the TestServer Service so it makes sense to just
				// always create it
				// the other benefit is this ensures that all tests start with a valid Proxy CR
				testServerVs,
			},
		}
	})

	JustBeforeEach(func() {
		err := snapshotWriter.WriteSnapshot(glooResources, clients.WriteOpts{
			Ctx:               ctx,
			OverwriteExisting: false,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	JustAfterEach(func() {
		err := snapshotWriter.DeleteSnapshot(glooResources, clients.DeleteOpts{
			Ctx:            ctx,
			IgnoreNotExist: true,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("tests with orphaned gateways", func() {

		It("correctly sets a status to a single gateway", func() {
			// Delete all VirtualServices to create an "orphaned" Gateway CR
			for _, vs := range glooResources.VirtualServices {
				err := resourceClientset.VirtualServiceClient().Delete(vs.GetMetadata().GetNamespace(), vs.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx, IgnoreNotExist: true})
				Expect(err).NotTo(HaveOccurred())
			}

			// demand that a created gateway _has_ a status.  This test is "good enough", as, prior to an orphaned gateway fix,
			// https://github.com/solo-io/gloo/pull/5790, free-floating gateways would never be assigned a status at all (nil)
			Eventually(func() *core.NamespacedStatuses {
				gw, err := resourceClientset.GatewayClient().Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				if err != nil {
					return nil
				}
				return gw.GetNamespacedStatuses()
			}, "15s", "0.5s").Should(Not(BeNil()))
		})
	})

	Context("Proxy reconciliation", func() {

		// This function parses a Proxy and determines how many routes are configured to point to the testserver service
		getRoutesToTestServer := func(proxy *gloov1.Proxy) int {
			routesToTestServer := 0
			for _, l := range proxy.Listeners {
				for _, vh := range utils.GetVirtualHostsForListener(l) {
					for _, r := range vh.Routes {
						if action := r.GetRouteAction(); action != nil {
							if single := action.GetSingle(); single != nil {
								if svcDest := single.GetKube(); svcDest != nil {
									if svcDest.Ref.Name == helper.TestServerName &&
										svcDest.Ref.Namespace == testHelper.InstallNamespace &&
										svcDest.Port == uint32(helper.TestServerPort) {
										routesToTestServer += 1
									}
								}
							}
						}
					}
				}
			}
			return routesToTestServer
		}

		It("should process proxy with deprecated label", func() {
			// wait for the expected proxy configuration to be accepted
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				proxy, err := resourceClientset.ProxyClient().Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				if err != nil {
					return nil, err
				}

				expectedRoutesToTestServer := 1 // we created a virtual service, with a single route to the testserver service
				actualRoutesToTestServer := getRoutesToTestServer(proxy)

				if expectedRoutesToTestServer != actualRoutesToTestServer {
					return nil, eris.Errorf("Expected %d routes to test server service, but found %d", expectedRoutesToTestServer, actualRoutesToTestServer)
				}
				return proxy, nil
			})

			// modify the proxy to use the deprecated label
			// this will simulate proxies that were persisted before the label change
			err := helpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Namespace: testHelper.InstallNamespace,
					Name:      defaults.GatewayProxyName,
				},
				func(resource resources.Resource) resources.Resource {
					proxy := resource.(*gloov1.Proxy)
					proxy.Metadata.Labels = map[string]string{
						"created_by": "gateway",
					}
					return proxy
				},
				resourceClientset.ProxyClient().BaseClient())
			Expect(err).NotTo(HaveOccurred())

			// modify the virtual service to trigger gateway reconciliation
			// any modification will work, for simplicity we duplicate a route on the virtual host
			err = helpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Namespace: testHelper.InstallNamespace,
					Name:      helper.TestServerName,
				},
				func(resource resources.Resource) resources.Resource {
					vs := resource.(*gatewayv1.VirtualService)
					vs.VirtualHost.Routes = append(vs.VirtualHost.Routes, vs.VirtualHost.Routes[0])
					return vs
				},
				resourceClientset.VirtualServiceClient().BaseClient())
			Expect(err).NotTo(HaveOccurred())

			// ensure that the changes from the virtual service are propagated to the proxy
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				proxy, err := resourceClientset.ProxyClient().Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				if err != nil {
					return nil, err
				}

				expectedRoutesToTestServer := 2 // we duplicated the route to the testserver service
				actualRoutesToTestServer := getRoutesToTestServer(proxy)

				if expectedRoutesToTestServer != actualRoutesToTestServer {
					return nil, eris.Errorf("Expected %d routes to test server service, but found %d", expectedRoutesToTestServer, actualRoutesToTestServer)
				}
				return proxy, nil
			})
		})
	})

	Context("tests with virtual service", func() {

		Context("CompressedProxySpec", Ordered, func() {

			AfterAll(func() {
				// Reset the CompressedProxySpec to False
				kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
					settings.GetGloo().DisableProxyGarbageCollection = &wrappers.BoolValue{Value: false}
					settings.GetGateway().CompressedProxySpec = false
				}, testHelper.InstallNamespace)

				// We delete the existing Proxy so that a new one can be auto-generated without compression
				err := resourceClientset.ProxyClient().Delete(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.DeleteOpts{
					Ctx:            ctx,
					IgnoreNotExist: true,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			DescribeTable("can route to upstream", func(compressedProxy bool) {
				kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
					settings.GetGloo().DisableProxyGarbageCollection = &wrappers.BoolValue{Value: false}
					settings.GetGateway().CompressedProxySpec = compressedProxy
				}, testHelper.InstallNamespace)

				// We delete the existing Proxy so that a new one can be auto-generated according to the `compressedSpec` definition
				err := resourceClientset.ProxyClient().Delete(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.DeleteOpts{
					Ctx:            ctx,
					IgnoreNotExist: true,
				})
				Expect(err).NotTo(HaveOccurred())

				// No-op patch on a resource to ensure translation re-sync's
				err = helpers.PatchResource(
					ctx,
					testServerVs.GetMetadata().Ref(),
					func(resource resources.Resource) resources.Resource {
						resource.GetMetadata().Annotations = map[string]string{
							"gloo-edge-test": "value",
						}
						return resource
					},
					resourceClientset.VirtualServiceClient().BaseClient())
				Expect(err).NotTo(HaveOccurred())

				// Assert that the generated Proxy matches the format we are testing (compressed or not)
				helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					proxy, err := resourceClientset.ProxyClient().Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{
						Ctx: ctx,
					})
					if compressedProxy {
						if proxy.GetMetadata().GetAnnotations()[compress.CompressedKey] != compress.CompressedValue {
							return nil, eris.New("Proxy should be compressed, but it does not contained compressed annotation")
						}
					} else {
						if proxy.GetMetadata().GetAnnotations()[compress.CompressedKey] != "" {
							return nil, eris.New("Proxy should not be compressed, but it does contain compressed annotation")
						}
					}
					return proxy, err
				})

				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              helper.TestServerName,
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1, // this is important, as sometimes curl hangs
					WithoutStats:      true,
					Verbose:           true,
				}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)
			},
				EntryDescription("can route to upstreams, compressedProxySpec = %v"),
				Entry(nil, false),
				Entry(nil, true))
		})

		Context("native ssl", func() {

			var secretName = "secret-native-ssl"

			BeforeEach(func() {
				// get the certificate so it is generated in the background
				go helpers.Certificate()

				tlsSecret := helpers.GetTlsSecret(secretName, testHelper.InstallNamespace)
				glooResources.Secrets = gloov1.SecretList{tlsSecret}

				// Modify the VirtualService to include the necessary SslConfig
				testServerVs.SslConfig = &ssl.SslConfig{
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: &core.ResourceRef{
							Name:      tlsSecret.GetMetadata().GetName(),
							Namespace: tlsSecret.GetMetadata().GetNamespace(),
						},
					},
				}
			})

			It("works with ssl", func() {
				caFile := kube2e.ToFile(helpers.Certificate())
				//goland:noinspection GoUnhandledErrorResult
				defer os.Remove(caFile)

				err := kubeCli.Copy(ctx, caFile, testHelper.InstallNamespace+"/testserver:/tmp/ca.crt")
				Expect(err).NotTo(HaveOccurred())

				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "https",
					Path:              "/",
					Method:            "GET",
					Host:              helper.TestServerName,
					Service:           defaults.GatewayProxyName,
					Port:              443,
					CaFile:            "/tmp/ca.crt",
					ConnectionTimeout: 1,
					WithoutStats:      true,
				}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)
			})
		})

		Context("linkerd enabled updates routes with appended headers", func() {

			var (
				httpEcho helper.TestContainer
			)

			BeforeEach(func() {
				var err error

				kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
					Expect(settings.GetGateway().GetCompressedProxySpec()).NotTo(BeNil())
					settings.Linkerd = true
				}, testHelper.InstallNamespace)

				httpEcho, err = helper.NewEchoHttp(testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred())

				err = httpEcho.DeployResources(2 * time.Minute)
				Expect(err).NotTo(HaveOccurred())

				httpEchoDestination := &gloov1.Destination{
					DestinationType: &gloov1.Destination_Upstream{
						Upstream: &core.ResourceRef{
							Namespace: testHelper.InstallNamespace,
							Name:      kubernetesplugin.UpstreamName(testHelper.InstallNamespace, helper.HttpEchoName, helper.HttpEchoPort),
						},
					},
				}
				httpEchoVs := helpers.NewVirtualServiceBuilder().
					WithName(helper.HttpEchoName).
					WithNamespace(testHelper.InstallNamespace).
					WithDomain(helper.HttpEchoName).
					WithRoutePrefixMatcher(helper.HttpEchoName, "/").
					WithRouteActionToSingleDestination(helper.HttpEchoName, httpEchoDestination).
					Build()

				glooResources.VirtualServices = []*gatewayv1.VirtualService{httpEchoVs}
			})

			AfterEach(func() {
				kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
					Expect(settings.GetGateway().GetCompressedProxySpec()).NotTo(BeNil())
					settings.Linkerd = true
				}, testHelper.InstallNamespace)

				err := httpEcho.TerminatePod()
				Expect(err).NotTo(HaveOccurred())

				// TODO: Terminate() should do this as part of its cleanup
				err = resourceClientset.ServiceClient().Delete(testHelper.InstallNamespace, helper.HttpEchoName, clients.DeleteOpts{Ctx: ctx, IgnoreNotExist: true})
				Expect(err).NotTo(HaveOccurred())
			})

			It("appends linkerd headers when linkerd is enabled", func() {
				responseString := fmt.Sprintf(`"%s":"%s.%s.svc.cluster.local:%v"`,
					linkerd.HeaderKey, helper.HttpEchoName, testHelper.InstallNamespace, helper.HttpEchoPort)
				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              helper.HttpEchoName,
					Service:           gatewayProxy,
					Port:              gatewayPort,
					ConnectionTimeout: 1,
					WithoutStats:      true,
				}, responseString, 1, 60*time.Second, 1*time.Second)
			})
		})

		Context("with a mix of valid and invalid routes on a single virtual service", func() {

			Context("route destination is nonexistent upstream", func() {

				BeforeEach(func() {
					// Add an invalid route to the testServer VirtualService
					// Prepend the route, since the other route is a catch all
					testServerVs.VirtualHost.Routes = append([]*gatewayv1.Route{
						{
							Name: "invalid-route",
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/invalid-route",
								},
							}},
							Action: &gatewayv1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Namespace: testHelper.InstallNamespace,
												Name:      "does-not-exist",
											},
										},
									}},
								},
							},
						},
					}, testServerVs.VirtualHost.Routes...)
				})

				It("serves a direct response for the invalid route response", func() {
					// the valid route should work
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/",
						Method:            "GET",
						Host:              helper.TestServerName,
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1, // this is important, as sometimes curl hangs
						WithoutStats:      true,
					}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)

					// the invalid route should respond with the direct response
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/invalid-route",
						Method:            "GET",
						Host:              helper.TestServerName,
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1, // this is important, as sometimes curl hangs
						WithoutStats:      true,
					}, &testmatchers.HttpResponse{
						Body:       ContainSubstring("Gloo Gateway has invalid configuration"),
						StatusCode: http.StatusNotFound,
					}, 1, 60*time.Second, 1*time.Second)
				})
			})

			Context("route prefix is invalid (ref delegation)", func() {

				BeforeEach(func() {
					goodRouteTable := getRouteTable("good-rt", nil, getRouteWithDirectResponse("Good response", "/route-1"))
					// bad RT's prefix does not start with parent's prefix, which should be a warning
					badRouteTable := getRouteTable("bad-rt", nil, getRouteWithDirectResponse("Bad response", "/does-not-match"))

					vsWithRouteTables := helpers.NewVirtualServiceBuilder().
						WithName("vs-with-route-tables").
						WithDomain("rt-delegation").
						WithNamespace(testHelper.InstallNamespace).
						WithRoutePrefixMatcher("good-route", "/route-1").
						WithRouteDelegateActionRef("good-route",
							&core.ResourceRef{
								Name:      "good-rt",
								Namespace: testHelper.InstallNamespace,
							}).
						WithRoutePrefixMatcher("bad-route", "/route-2").
						WithRouteDelegateActionRef("bad-route",
							&core.ResourceRef{
								Name:      "bad-rt",
								Namespace: testHelper.InstallNamespace,
							}).
						Build()

					glooResources.VirtualServices = []*gatewayv1.VirtualService{vsWithRouteTables}
					glooResources.RouteTables = []*gatewayv1.RouteTable{
						goodRouteTable,
						badRouteTable,
					}
				})

				It("invalid route delegated via ref does not prevent updates to valid routes", func() {
					// the good RT should be accepted, but both the VS and bad RT should have a warning
					helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
						return resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, "vs-with-route-tables", clients.ReadOpts{Ctx: ctx})
					})
					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return resourceClientset.RouteTableClient().Read(testHelper.InstallNamespace, "good-rt", clients.ReadOpts{Ctx: ctx})
					})
					helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
						return resourceClientset.RouteTableClient().Read(testHelper.InstallNamespace, "bad-rt", clients.ReadOpts{Ctx: ctx})
					})

					// the valid route should return the expected direct response
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/route-1",
						Method:            "GET",
						Host:              "rt-delegation",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, "Good response", 1, 60*time.Second, 1*time.Second)

					// the invalid route should return a 404
					res, err := testHelper.Curl(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/route-2",
						Method:            "GET",
						Host:              "rt-delegation",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
						ReturnHeaders:     true,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(res).To(ContainSubstring("404 Not Found"))

					// update the response of the good RT
					err = helpers.PatchResource(
						ctx,
						&core.ResourceRef{
							Namespace: testHelper.InstallNamespace,
							Name:      "good-rt",
						},
						func(resource resources.Resource) resources.Resource {
							rt := resource.(*gatewayv1.RouteTable)
							rt.Routes[0].Action = &gatewayv1.Route_DirectResponseAction{
								DirectResponseAction: &gloov1.DirectResponseAction{
									Status: 200,
									Body:   "Updated good response",
								},
							}
							return rt
						},
						resourceClientset.RouteTableClient().BaseClient(),
					)
					Expect(err).NotTo(HaveOccurred())

					// make sure it returns the new response
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/route-1",
						Method:            "GET",
						Host:              "rt-delegation",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, "Updated good response", 1, 60*time.Second, 1*time.Second)
				})

			})

			Context("route prefix is invalid (selector delegation)", func() {

				BeforeEach(func() {
					labels := map[string]string{"select": "me"}
					goodRouteTable := getRouteTable("good-rt", labels, getRouteWithDirectResponse("Good response", "/delegate/route-1"))
					// bad RT's prefix does not start with parent's prefix, which should be a warning
					badRouteTable := getRouteTable("bad-rt", labels, getRouteWithDirectResponse("Bad response", "/does-not-match"))

					vsWithRouteTables := helpers.NewVirtualServiceBuilder().
						WithName("vs-with-route-tables").
						WithDomain("rt-delegation").
						WithNamespace(testHelper.InstallNamespace).
						WithRoutePrefixMatcher("good-route", "/delegate").
						WithRouteDelegateActionSelector("good-route",
							&gatewayv1.RouteTableSelector{
								Namespaces: []string{testHelper.InstallNamespace},
								Labels:     labels,
							}).
						Build()

					glooResources.VirtualServices = []*gatewayv1.VirtualService{vsWithRouteTables}
					glooResources.RouteTables = []*gatewayv1.RouteTable{
						goodRouteTable,
						badRouteTable,
					}
				})

				It("invalid route delegated via selector does not prevent updates to valid routes", func() {
					// the good RT should be accepted, but both the VS and bad RT should have a warning
					helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
						return resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, "vs-with-route-tables", clients.ReadOpts{Ctx: ctx})
					})
					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return resourceClientset.RouteTableClient().Read(testHelper.InstallNamespace, "good-rt", clients.ReadOpts{Ctx: ctx})
					})
					helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
						return resourceClientset.RouteTableClient().Read(testHelper.InstallNamespace, "bad-rt", clients.ReadOpts{Ctx: ctx})
					})

					By("the valid route should return the expected direct response")
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/delegate/route-1",
						Method:            "GET",
						Host:              "rt-delegation",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, "Good response", 1, 60*time.Second, 1*time.Second)

					By("the RT should be updated to return a direct response")
					err := helpers.PatchResource(
						ctx,
						&core.ResourceRef{
							Namespace: testHelper.InstallNamespace,
							Name:      "good-rt",
						},
						func(resource resources.Resource) resources.Resource {
							rt := resource.(*gatewayv1.RouteTable)
							rt.Routes[0].Action = &gatewayv1.Route_DirectResponseAction{
								DirectResponseAction: &gloov1.DirectResponseAction{
									Status: 200,
									Body:   "Updated good response",
								},
							}
							return rt
						},
						resourceClientset.RouteTableClient().BaseClient(),
					)
					Expect(err).NotTo(HaveOccurred())

					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/delegate/route-1",
						Method:            "GET",
						Host:              "rt-delegation",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, "Updated good response", 1, 60*time.Second, 1*time.Second)
				})
			})
		})

		Context("proxy debug endpoint", func() {

			It("Returns proxies", func() {
				portForwarder, err := kubeCli.StartPortForward(ctx,
					portforward.WithDeployment(kubeutils.GlooDeploymentName, testHelper.InstallNamespace),
					portforward.WithRemotePort(defaults2.GlooProxyDebugPort),
				)
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					portForwarder.Close()
					portForwarder.WaitForStop()
				}()

				cc, err := grpc.DialContext(ctx, portForwarder.Address(), grpc.WithInsecure())
				Expect(err).NotTo(HaveOccurred())
				debugClient := debug.NewProxyEndpointServiceClient(cc)

				Eventually(func() error {
					referenceProxy, err := resourceClientset.ProxyClient().Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
					if err != nil {
						return err
					}
					resp, err := debugClient.GetProxies(ctx, &debug.ProxyEndpointRequest{Namespace: testHelper.InstallNamespace, Name: defaults.GatewayProxyName})
					if err != nil {
						return err
					}
					fmt.Sprintf("response %v", resp)
					if len(resp.GetProxies()) != 1 {
						return eris.Errorf("Expected to find 1 proxy, found %d", len(resp.GetProxies()))
					}
					if !resp.GetProxies()[0].Equal(referenceProxy) {
						return eris.Errorf("Expected the proxy from the debug endpoint to equal the proxy from proxyClient")
					}
					return nil
				}, "10s", "1s").ShouldNot(HaveOccurred())
			})
		})
	})

	Context("tests with route tables", func() {

		BeforeEach(func() {
			rt2 := getRouteTable("rt2", nil, getRouteWithDest(testServerDestination, "/root/rt1/rt2"))
			rt1 := getRouteTable("rt1", nil, getRouteWithDelegateRef(rt2.Metadata.Name, "/root/rt1"))

			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-with-rt").
				WithDomain("rt-delegation").
				WithNamespace(testHelper.InstallNamespace).
				WithRouteDelegateActionRef("route",
					&core.ResourceRef{
						Name:      rt1.GetMetadata().GetName(),
						Namespace: testHelper.InstallNamespace,
					}).
				WithRoutePrefixMatcher("route", "/root").
				WithRouteOptions("route", &gloov1.RouteOptions{PrefixRewrite: &wrappers.StringValue{Value: "/"}}).
				Build()

			glooResources.VirtualServices = []*gatewayv1.VirtualService{vs}
			glooResources.RouteTables = []*gatewayv1.RouteTable{
				rt1,
				rt2,
			}
		})

		It("correctly routes requests to an upstream", func() {
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/root/rt1/rt2",
				Method:            "GET",
				Host:              "rt-delegation",
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1, // this is important, as sometimes curl hangs
				WithoutStats:      true,
			}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)
		})
	})

	Context("tests with VirtualHostOptions", func() {

		BeforeEach(func() {
			vh1 := &gatewayv1.VirtualHostOption{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "vh-opt-one",
				},
				Options: &gloov1.VirtualHostOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToRemove: []string{"header-from-external-options1"},
					},
					Cors: &cors.CorsPolicy{
						ExposeHeaders: []string{"header-from-extopt1"},
						AllowOrigin:   []string{"some-origin-1"},
					},
				},
			}
			vh2 := &gatewayv1.VirtualHostOption{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "vh-opt-two",
				},
				Options: &gloov1.VirtualHostOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToRemove: []string{"header-from-external-options2"},
					},
					Cors: &cors.CorsPolicy{
						ExposeHeaders: []string{"header-from-extopt2"},
						AllowOrigin:   []string{"some-origin-2"},
					},
					Transformations: &glootransformation.Transformations{
						RequestTransformation: &glootransformation.Transformation{
							TransformationType: &glootransformation.Transformation_TransformationTemplate{
								TransformationTemplate: &glootransformation.TransformationTemplate{
									Headers: map[string]*glootransformation.InjaTemplate{
										"x-header-added-in-opt2": {
											Text: "this header was added in the VirtualHostOption object vhOpt2",
										},
									},
								},
							},
						},
					},
				},
			}

			vs := &gatewayv1.VirtualService{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "vs",
				},
				VirtualHost: &gatewayv1.VirtualHost{
					Domains: []string{"*"},
					Routes:  []*gatewayv1.Route{getRouteWithDest(testServerDestination, "/")},
					Options: &gloov1.VirtualHostOptions{
						HeaderManipulation: &headers.HeaderManipulation{
							RequestHeadersToRemove: []string{"header-from-vhost"},
						},
					},
					ExternalOptionsConfig: &gatewayv1.VirtualHost_OptionsConfigRefs{
						OptionsConfigRefs: &gatewayv1.DelegateOptionsRefs{
							DelegateOptions: []*core.ResourceRef{
								{
									Namespace: testHelper.InstallNamespace,
									Name:      "vh-opt-one",
								},
								{
									Namespace: testHelper.InstallNamespace,
									Name:      "vh-opt-two",
								},
							},
						},
					},
				},
			}

			glooResources.VirtualServices = gatewayv1.VirtualServiceList{vs}
			glooResources.VirtualHostOptions = gatewayv1.VirtualHostOptionList{
				vh1,
				vh2,
			}
		})

		It("correctly delegates options from VirtualHostOption", func() {
			Eventually(func(g Gomega) {
				// https://onsi.github.io/gomega/#category-3-making-assertions-eminem-the-function-passed-into-codeeventuallycode
				getProxy := func() (resources.InputResource, error) {
					return resourceClientset.ProxyClient().Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				}

				proxyInputResource, err := getProxy()
				g.Expect(err).NotTo(HaveOccurred())
				proxy := proxyInputResource.(*gloov1.Proxy)

				for _, l := range proxy.Listeners {
					httpListener := l.GetHttpListener()
					if httpListener == nil {
						continue
					}
					for _, vhost := range httpListener.GetVirtualHosts() {
						opts := vhost.GetOptions()

						// option config on VirtualHost overrides all delegated options
						vsHeaderManipulation := &headers.HeaderManipulation{
							RequestHeadersToRemove: []string{"header-from-vhost"},
						}
						g.Expect(opts.GetHeaderManipulation()).To(gloo_matchers.MatchProto(vsHeaderManipulation))

						// since rt1 is delegated to first, it overrides rt2, which was delegated later
						vhost1Cors := &cors.CorsPolicy{
							ExposeHeaders: []string{"header-from-extopt1"},
							AllowOrigin:   []string{"some-origin-1"},
						}
						g.Expect(opts.GetCors()).To(gloo_matchers.MatchProto(vhost1Cors))

						// options that weren't already set in previously delegated options are set from rt2
						vhost2Transformations := &glootransformation.Transformations{
							RequestTransformation: &glootransformation.Transformation{
								TransformationType: &glootransformation.Transformation_TransformationTemplate{
									TransformationTemplate: &glootransformation.TransformationTemplate{
										Headers: map[string]*glootransformation.InjaTemplate{
											"x-header-added-in-opt2": {
												Text: "this header was added in the VirtualHostOption object vhOpt2",
											},
										},
									},
								},
							},
						}
						g.Expect(opts.GetTransformations()).To(gloo_matchers.MatchProto(vhost2Transformations))
					}
				}

				// Confirm that the Resource is accepted as well
				// If the Proxy has the necessary values, but the resource has been rejected, this test is not behaving
				// properly and should fail
				helpers.EventuallyResourceAccepted(getProxy)

			}, "30s", "1s").Should(Succeed())
		})
	})

	Context("tests with RouteOptions", func() {

		BeforeEach(func() {
			rt1 := &gatewayv1.RouteOption{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "rt-opt-one",
				},
				Options: &gloov1.RouteOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToRemove: []string{"header-from-external-options1"},
					},
					Cors: &cors.CorsPolicy{
						ExposeHeaders: []string{"header-from-extopt1"},
						AllowOrigin:   []string{"some-origin-1"},
					},
				},
			}
			rt2 := &gatewayv1.RouteOption{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "rt-opt-two",
				},
				Options: &gloov1.RouteOptions{
					HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToRemove: []string{"header-from-external-options2"},
					},
					Cors: &cors.CorsPolicy{
						ExposeHeaders: []string{"header-from-extopt2"},
						AllowOrigin:   []string{"some-origin-2"},
					},
					Transformations: &glootransformation.Transformations{
						RequestTransformation: &glootransformation.Transformation{
							TransformationType: &glootransformation.Transformation_TransformationTemplate{
								TransformationTemplate: &glootransformation.TransformationTemplate{
									Headers: map[string]*glootransformation.InjaTemplate{
										"x-header-added-in-opt2": {
											Text: "this header was added in the VirtualHostOption object vhOpt2",
										},
									},
								},
							},
						},
					},
				},
			}

			vs := &gatewayv1.VirtualService{
				Metadata: &core.Metadata{
					Namespace: testHelper.InstallNamespace,
					Name:      "vs",
				},
				VirtualHost: &gatewayv1.VirtualHost{
					Domains: []string{"*"},
					Routes: []*gatewayv1.Route{
						{
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/",
								},
							}},
							Action: &gatewayv1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: testServerDestination,
									},
								},
							},
							Options: &gloov1.RouteOptions{
								HeaderManipulation: &headers.HeaderManipulation{
									RequestHeadersToRemove: []string{"header-from-vhost"},
								},
							},
							ExternalOptionsConfig: &gatewayv1.Route_OptionsConfigRefs{
								OptionsConfigRefs: &gatewayv1.DelegateOptionsRefs{
									DelegateOptions: []*core.ResourceRef{
										{
											Namespace: testHelper.InstallNamespace,
											Name:      "rt-opt-one",
										},
										{
											Namespace: testHelper.InstallNamespace,
											Name:      "rt-opt-two",
										},
									},
								},
							},
						},
					},
				},
			}

			glooResources.VirtualServices = gatewayv1.VirtualServiceList{vs}
			glooResources.RouteOptions = gatewayv1.RouteOptionList{
				rt1,
				rt2,
			}
		})

		It("correctly delegates options from RouteOption", func() {
			Eventually(func(g Gomega) {
				// https://onsi.github.io/gomega/#category-3-making-assertions-eminem-the-function-passed-into-codeeventuallycode
				getProxy := func() (resources.InputResource, error) {
					return resourceClientset.ProxyClient().Read(testHelper.InstallNamespace, defaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				}

				proxyInputResource, err := getProxy()
				g.Expect(err).NotTo(HaveOccurred())
				proxy := proxyInputResource.(*gloov1.Proxy)

				for _, l := range proxy.Listeners {
					httpListener := l.GetHttpListener()
					if httpListener == nil {
						continue
					}
					for _, vhost := range httpListener.GetVirtualHosts() {
						for _, route := range vhost.GetRoutes() {
							opts := route.GetOptions()

							// option config on VirtualHost overrides all delegated options
							vsHeaderManipulation := &headers.HeaderManipulation{
								RequestHeadersToRemove: []string{"header-from-vhost"},
							}
							g.Expect(opts.GetHeaderManipulation()).To(gloo_matchers.MatchProto(vsHeaderManipulation))

							// since rt1 is delegated to first, it overrides rt2, which was delegated later
							rt1Cors := &cors.CorsPolicy{
								ExposeHeaders: []string{"header-from-extopt1"},
								AllowOrigin:   []string{"some-origin-1"},
							}
							g.Expect(opts.GetCors()).To(gloo_matchers.MatchProto(rt1Cors))

							// options that weren't already set in previously delegated options are set from rt2
							rt2Transformation := &glootransformation.Transformations{
								RequestTransformation: &glootransformation.Transformation{
									TransformationType: &glootransformation.Transformation_TransformationTemplate{
										TransformationTemplate: &glootransformation.TransformationTemplate{
											Headers: map[string]*glootransformation.InjaTemplate{
												"x-header-added-in-opt2": {
													Text: "this header was added in the VirtualHostOption object vhOpt2",
												},
											},
										},
									},
								},
							}
							g.Expect(opts.GetTransformations()).To(gloo_matchers.MatchProto(rt2Transformation))
						}
					}
				}

				// Confirm that the Resource is accepted as well
				// If the Proxy has the necessary values, but the resource has been rejected, this test is not behaving
				// properly and should fail
				helpers.EventuallyResourceAccepted(getProxy)

			}, "30s", "1s").Should(Succeed())
		})
	})

	Context("upstream discovery", func() {
		var createdServices []string

		var createServicesForPod = func(displayName string, port int32) {
			createdServices = nil
			// create some services
			for i := range 20 {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:   fmt.Sprintf("%s-%d", displayName, i),
						Labels: map[string]string{"gloo": displayName},
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"gloo": displayName},
						Ports: []corev1.ServicePort{{
							Port: port,
						}},
					},
				}
				service, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Create(ctx, service, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				createdServices = append(createdServices, service.Name)
			}
		}

		createServiceWithWatchedLabels := func(svcName string, watchedLabels map[string]string) {
			// merge watchedLabels into service labels
			labels := map[string]string{"gloo": svcName}
			for key, val := range watchedLabels {
				labels[key] = val
			}
			// Write service
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:   svcName,
					Labels: labels,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"gloo": svcName,
					},
					Ports: []corev1.ServicePort{{
						Port: helper.TestServerPort,
					}},
				},
			}
			service, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Create(ctx, service, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			createdServices = append(createdServices, service.Name)
		}

		getUpstream := func(svcname string) (*gloov1.Upstream, error) {
			upstreamName := kubernetesplugin.UpstreamName(testHelper.InstallNamespace, svcname, helper.TestServerPort)
			return resourceClientset.UpstreamClient().Read(testHelper.InstallNamespace, upstreamName, clients.ReadOpts{})
		}

		// Update the Gloo Discovery WatchLabels setting to the specified value
		setWatchLabels := func(watchLabels map[string]string) {
			kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
				settings.GetDiscovery().UdsOptions = &gloov1.Settings_DiscoveryOptions_UdsOptions{
					WatchLabels: watchLabels,
				}
			}, testHelper.InstallNamespace)
		}
		AfterEach(func() {
			for _, svcName := range createdServices {
				_ = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, svcName, metav1.DeleteOptions{})
			}

			setWatchLabels(nil)
		})

		It("should preserve discovery", FlakeAttempts(5), func() {
			// This test has flaked before with the following error:
			// 	Failed to validate Proxy [namespace: gloo-system, name: gateway-proxy] with gloo validation:
			//	Listener Error: SSLConfigError. Reason: SSL secret not found: list did not find secret gloo-system.secret-native-ssl\n\n",
			// This seems to be the result of test pollution since the secret is created in a separate test
			// This has only caused this test, which depends on discovery to flake, so in the meantime, we are adding
			// a flake decorator

			createServicesForPod(helper.TestServerName, helper.TestServerPort)

			for _, svc := range createdServices {
				Eventually(func() (*gloov1.Upstream, error) {
					return getUpstream(svc)
				}, "15s", "0.5s").ShouldNot(BeNil())

				// now set subset config on an upstream:
				err := helpers.PatchResource(
					ctx,
					&core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      kubernetesplugin.UpstreamName(testHelper.InstallNamespace, svc, helper.TestServerPort),
					},
					func(resource resources.Resource) resources.Resource {
						upstream := resource.(*gloov1.Upstream)
						upstream.UpstreamType.(*gloov1.Upstream_Kube).Kube.ServiceSpec = &gloov1plugins.ServiceSpec{
							PluginType: &gloov1plugins.ServiceSpec_Grpc{
								Grpc: &grpcv1.ServiceSpec{},
							},
						}
						return upstream
					},
					resourceClientset.UpstreamClient().BaseClient())
				Expect(err).NotTo(HaveOccurred())
			}

			// chill for a few letting discovery reconcile
			time.Sleep(time.Second * 10)

			// validate that all subset settings are still there
			for _, svc := range createdServices {
				// now set subset config on an upstream:
				up, _ := getUpstream(svc)
				spec := up.GetKube().GetServiceSpec()
				Expect(spec).ToNot(BeNil())
				Expect(spec.GetGrpc()).ToNot(BeNil())
			}
		})

		It("Discovers upstream with label that matches watched labels", func() {
			watchedKey := "A"
			watchedValue := "B"
			watchedLabels := map[string]string{watchedKey: watchedValue}
			setWatchLabels(watchedLabels)

			svcName := "uds-test-service"
			createServiceWithWatchedLabels(svcName, watchedLabels)

			Eventually(func() (*gloov1.Upstream, error) {
				return getUpstream(svcName)
			}, "15s", "0.5s").ShouldNot(BeNil())
		})

		It("Does not discover upstream with no label when watched labels are set", func() {
			watchedKey := "A"
			watchedValue := "B"
			watchedLabels := map[string]string{watchedKey: watchedValue}
			setWatchLabels(watchedLabels)

			svcName := "uds-test-service"
			createServiceWithWatchedLabels(svcName, nil)

			Consistently(func() error {
				_, err := getUpstream(svcName)
				return err
			}, "15s", "0.5s").Should(HaveOccurred())
		})

		It("Does not discover upstream with mismatched label value", func() {
			watchedKey := "A"
			watchedValue := "B"
			unwatchedValue := "C"
			watchedLabels := map[string]string{watchedKey: watchedValue}
			setWatchLabels(watchedLabels)

			svcName := "uds-test-service"
			unwatchedLabels := map[string]string{watchedKey: unwatchedValue}
			createServiceWithWatchedLabels(svcName, unwatchedLabels)

			Consistently(func() error {
				_, err := getUpstream(svcName)
				return err
			}, "15s", "0.5s").Should(HaveOccurred())
		})
	})

	Context("tcp", func() {

		var (
			httpEcho            helper.TestContainer
			httpEchoClusterName string
			curlPod             helper.TestContainer
			clusterIp           string
			tcpPort             = corev1.ServicePort{
				Name:       "tcp-proxy",
				Port:       int32(defaults2.TcpPort),
				TargetPort: intstr.FromInt(int(defaults2.TcpPort)),
				Protocol:   "TCP",
			}
		)

		BeforeEach(func() {
			var err error

			httpEcho, err = helper.NewEchoHttp(testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred())

			err = httpEcho.DeployResources(time.Minute)
			Expect(err).NotTo(HaveOccurred())

			curlPod, err = helper.NewCurl(testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred())

			err = curlPod.DeployResources(time.Minute)
			Expect(err).NotTo(HaveOccurred())

			gwSvc, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Get(ctx, gatewayProxy, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			clusterIp = gwSvc.Spec.ClusterIP
			found := false
			for _, v := range gwSvc.Spec.Ports {
				if v.Name == tcpPort.Name || v.Port == tcpPort.Port {
					found = true
					break
				}
			}
			if !found {
				gwSvc.Spec.Ports = append(gwSvc.Spec.Ports, tcpPort)
			}
			_, err = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Update(ctx, gwSvc, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			httpEchoClusterName = translator.UpstreamToClusterName(&core.ResourceRef{
				Namespace: testHelper.InstallNamespace,
				Name:      kubernetesplugin.UpstreamName(testHelper.InstallNamespace, helper.HttpEchoName, helper.HttpEchoPort),
			})
		})

		AfterEach(func() {
			gwSvc, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Get(ctx, gatewayProxy, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			ports := make([]corev1.ServicePort, 0, len(gwSvc.Spec.Ports))
			for _, v := range gwSvc.Spec.Ports {
				if v.Name != tcpPort.Name {
					ports = append(ports, v)
				}
			}
			gwSvc.Spec.Ports = ports
			_, err = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Update(ctx, gwSvc, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			err = httpEcho.TerminatePod()
			Expect(err).NotTo(HaveOccurred())

			err = curlPod.TerminatePod()
			Expect(err).NotTo(HaveOccurred())

			err = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, helper.HttpEchoName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("routing (tcp)", func() {

			BeforeEach(func() {
				tcpGateway := defaults.DefaultTcpGateway(testHelper.InstallNamespace)
				tcpGateway.GetTcpGateway().TcpHosts = []*gloov1.TcpHost{{
					Name: "one",
					Destination: &gloov1.TcpHost_TcpAction{
						Destination: &gloov1.TcpHost_TcpAction_Single{
							Single: &gloov1.Destination{
								DestinationType: &gloov1.Destination_Kube{
									Kube: &gloov1.KubernetesServiceDestination{
										Ref: &core.ResourceRef{
											Name:      helper.HttpEchoName,
											Namespace: testHelper.InstallNamespace,
										},
										Port: uint32(helper.HttpEchoPort),
									},
								},
							},
						},
					},
				}}

				glooResources.Gateways = gatewayv1.GatewayList{tcpGateway}
			})

			It("correctly routes to the service (tcp)", func() {
				responseString := fmt.Sprintf(`"hostname":"%s"`, gatewayProxy)

				curlPod.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Service:           gatewayProxy,
					Port:              int(defaults2.TcpPort),
					ConnectionTimeout: 1,
				}, responseString, 0, 30*time.Second)
			})

		})

		Context("routing (tcp/tls)", func() {

			const (
				secretName  = "secret-routing-tls"
				gatewayName = "one"
			)

			BeforeEach(func() {
				// Create secret to use for ssl routing
				tlsSecret := helpers.GetTlsSecret(secretName, testHelper.InstallNamespace)
				glooResources.Secrets = gloov1.SecretList{tlsSecret}

				tcpGateway := defaults.DefaultTcpGateway(testHelper.InstallNamespace)
				tcpGateway.GetTcpGateway().TcpHosts = []*gloov1.TcpHost{{
					Name: gatewayName,
					Destination: &gloov1.TcpHost_TcpAction{
						Destination: &gloov1.TcpHost_TcpAction_ForwardSniClusterName{
							ForwardSniClusterName: &empty.Empty{},
						},
					},
					SslConfig: &ssl.SslConfig{
						// Use the translated cluster name as the SNI domain so envoy uses that in the cluster field
						SniDomains: []string{httpEchoClusterName},
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      tlsSecret.GetMetadata().GetName(),
								Namespace: tlsSecret.GetMetadata().GetNamespace(),
							},
						},
						// Force http1, as defaulting to 2 fails. The service in question is an http1 service, but as this
						// is a standard TCP connection envoy does not know that, so it must rely on ALPN to figure that out.
						// However, by default the ALPN is set to []string{"h2", "http/1.1"} which favors http2.
						AlpnProtocols: []string{"http/1.1"},
					},
				}}

				glooResources.Gateways = gatewayv1.GatewayList{tcpGateway}

			})

			It("correctly routes to the service (tcp/tls)", func() {
				responseString := fmt.Sprintf(`"hostname":"%s"`, httpEchoClusterName)

				curlPod.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "https",
					Sni:               httpEchoClusterName,
					Service:           clusterIp,
					Port:              int(defaults2.TcpPort),
					ConnectionTimeout: 1,
					SelfSigned:        true,
				}, responseString, 0, 30*time.Second)
			})

		})

	})

	Context("with subsets and upstream groups", func() {

		var (
			redPod   *corev1.Pod
			bluePod  *corev1.Pod
			greenPod *corev1.Pod
			service  *corev1.Service

			upstreamName string
		)

		BeforeEach(func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "pod",
					Labels:       map[string]string{"app": "redblue", "text": "red"},
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: pointerToInt64(0),
					Containers: []corev1.Container{{
						Name:  "echo",
						Image: kube2e.GetHttpEchoImage(),
						Args:  []string{"-text=\"red-pod\""},
					}},
				}}
			var err error
			redPod, err = resourceClientset.KubeClients().CoreV1().Pods(testHelper.InstallNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			pod.Labels["text"] = "blue"
			pod.Spec.Containers[0].Args = []string{"-text=\"blue-pod\""}
			bluePod, err = resourceClientset.KubeClients().CoreV1().Pods(testHelper.InstallNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// green pod - no label
			delete(pod.Labels, "text")
			pod.Spec.Containers[0].Args = []string{"-text=\"green-pod\""}
			greenPod, err = resourceClientset.KubeClients().CoreV1().Pods(testHelper.InstallNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			service = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "redblue",
					Labels:       map[string]string{"app": "redblue"},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "redblue"},
					Ports: []corev1.ServicePort{{
						Port: 5678,
					}},
				},
			}
			service, err = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Create(ctx, service, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			upstreamName = kubernetesplugin.UpstreamName(testHelper.InstallNamespace, service.Name, 5678)
			// wait for upstream to get created by discovery
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return resourceClientset.UpstreamClient().Read(testHelper.InstallNamespace, upstreamName, clients.ReadOpts{Ctx: ctx})
			})
			// add subset spec to upstream
			err = helpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Namespace: testHelper.InstallNamespace,
					Name:      upstreamName,
				},
				func(resource resources.Resource) resources.Resource {
					us := resource.(*gloov1.Upstream)
					us.UpstreamType.(*gloov1.Upstream_Kube).Kube.SubsetSpec = &gloov1plugins.SubsetSpec{
						Selectors: []*gloov1plugins.Selector{{
							Keys: []string{"text"},
						}},
					}
					return us
				},
				resourceClientset.UpstreamClient().BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			upstreamRef := &core.ResourceRef{
				Name:      upstreamName,
				Namespace: testHelper.InstallNamespace,
			}

			ug := &gloov1.UpstreamGroup{
				Metadata: &core.Metadata{
					Name:      "test",
					Namespace: testHelper.InstallNamespace,
				},
				Destinations: []*gloov1.WeightedDestination{
					{
						Weight: &wrappers.UInt32Value{Value: 1},
						Destination: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: upstreamRef,
							},
							Subset: &gloov1.Subset{
								Values: map[string]string{"text": "red"},
							},
						},
					},
					{
						Weight: &wrappers.UInt32Value{Value: 1},
						Destination: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: upstreamRef,
							},
							Subset: &gloov1.Subset{
								Values: map[string]string{"text": "blue"},
							},
						},
					},
					{
						Weight: &wrappers.UInt32Value{Value: 1},
						Destination: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: upstreamRef,
							},
							Subset: &gloov1.Subset{
								Values: map[string]string{"text": ""},
							},
						},
					},
				},
			}

			vs := &gatewayv1.VirtualService{
				Metadata: &core.Metadata{
					Name:      "vs",
					Namespace: testHelper.InstallNamespace,
				},
				VirtualHost: &gatewayv1.VirtualHost{
					Domains: []string{"*"},
					Routes: []*gatewayv1.Route{
						{
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/red",
								},
							}},
							Action: &gatewayv1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: upstreamRef,
											},
											Subset: &gloov1.Subset{
												Values: map[string]string{"text": "red"},
											},
										},
									},
								},
							},
						}, {
							Action: &gatewayv1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_UpstreamGroup{
										UpstreamGroup: ug.GetMetadata().Ref(),
									},
								},
							},
						},
					},
				},
			}

			glooResources.VirtualServices = gatewayv1.VirtualServiceList{vs}
			glooResources.UpstreamGroups = gloov1.UpstreamGroupList{ug}
		})

		AfterEach(func() {
			if redPod != nil {
				err := resourceClientset.KubeClients().CoreV1().Pods(testHelper.InstallNamespace).Delete(ctx, redPod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}
			if bluePod != nil {
				err := resourceClientset.KubeClients().CoreV1().Pods(testHelper.InstallNamespace).Delete(ctx, bluePod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}
			if greenPod != nil {
				err := resourceClientset.KubeClients().CoreV1().Pods(testHelper.InstallNamespace).Delete(ctx, greenPod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}
			if service != nil {
				err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Delete(ctx, service.Name, metav1.DeleteOptions{GracePeriodSeconds: pointerToInt64(0)})
				Expect(err).NotTo(HaveOccurred())
			}

			// Ensure the redblue service is deleted
			helpers.EventuallyObjectDeleted(func() (client.Object, error) {
				return resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Get(ctx, service.Name, metav1.GetOptions{})
			}, "15s", ".5s")

			Eventually(func() error {
				coloredPods, err := resourceClientset.KubeClients().CoreV1().Pods(testHelper.InstallNamespace).List(ctx,
					metav1.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{"app": "redblue"}).String()})
				if err != nil {
					return err
				}
				vsList, err := resourceClientset.VirtualServiceClient().List(testHelper.InstallNamespace, clients.ListOpts{Ctx: ctx})
				if err != nil {
					return err
				}
				// After we remove the virtual service, the proxy should be removed as well by the gateway controller
				proxyList, err := resourceClientset.ProxyClient().List(testHelper.InstallNamespace, clients.ListOpts{Ctx: ctx})
				if err != nil {
					return err
				}

				if len(coloredPods.Items)+len(vsList)+len(proxyList) == 0 {
					return nil
				}
				return eris.Errorf("expected all test resources to have been deleted but found: "+
					"%d pods, %d virtual services, %d proxies", len(coloredPods.Items), len(vsList), len(proxyList))
			}, time.Minute, time.Second).Should(BeNil())
		})

		It("routes to subsets and upstream groups", func() {

			// make sure we get all three upstreams:
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, "red-pod", 1, 120*time.Second, 1*time.Second)

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, "blue-pod", 1, 120*time.Second, 1*time.Second)

			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, "green-pod", 1, 120*time.Second, 1*time.Second)

			// now make sure we only get the red pod
			redOpts := helper.CurlOpts{
				Protocol: "http",
				Path:     "/red",
				Method:   "GET",
				Host:     gatewayProxy,
				Service:  gatewayProxy,
				Port:     gatewayPort,
				// This value matches our RetryMaxTime
				ConnectionTimeout: 5,
				WithoutStats:      true,
				// These redOpts are used in a curl that is expected to consistently pass
				// We rely on curl retries to prevent network flakes from causing test flakes
				Retries: struct {
					Retry        int
					RetryDelay   int
					RetryMaxTime int
				}{Retry: 3, RetryDelay: 0, RetryMaxTime: 5},
			}

			// try it 10 times
			for range 10 {
				res, err := testHelper.Curl(redOpts)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(ContainSubstring("red-pod"))
			}
		})
	})

	Context("matchable hybrid gateway", func() {

		var (
			hybridProxyServicePort = corev1.ServicePort{
				Name:       "hybrid-proxy",
				Port:       int32(defaults2.HybridPort),
				TargetPort: intstr.FromInt(int(defaults2.HybridPort)),
				Protocol:   "TCP",
			}
		)

		exposePortOnGwProxyService := func() {
			gwSvc, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Get(ctx, gatewayProxy, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Append servicePort if not found already
			found := false
			for _, v := range gwSvc.Spec.Ports {
				if v.Name == hybridProxyServicePort.Name || v.Port == hybridProxyServicePort.Port {
					found = true
					break
				}
			}
			if !found {
				gwSvc.Spec.Ports = append(gwSvc.Spec.Ports, hybridProxyServicePort)
			}

			_, err = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Update(ctx, gwSvc, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			exposePortOnGwProxyService()

			// Create a MatchableHttpGateway
			matchableHttpGateway := &gatewayv1.MatchableHttpGateway{
				Metadata: &core.Metadata{
					Name:      "matchable-http-gateway",
					Namespace: testHelper.InstallNamespace,
				},
				HttpGateway: &gatewayv1.HttpGateway{
					// match all virtual services
				},
			}

			// Create a HybridGateway that references that MatchableHttpGateway
			hybridGateway := &gatewayv1.Gateway{
				Metadata: &core.Metadata{
					Name:      fmt.Sprintf("%s-hybrid", defaults.GatewayProxyName),
					Namespace: testHelper.InstallNamespace,
				},
				GatewayType: &gatewayv1.Gateway_HybridGateway{
					HybridGateway: &gatewayv1.HybridGateway{
						DelegatedHttpGateways: &gatewayv1.DelegatedHttpGateway{
							SelectionType: &gatewayv1.DelegatedHttpGateway_Ref{
								Ref: &core.ResourceRef{
									Name:      matchableHttpGateway.GetMetadata().GetName(),
									Namespace: matchableHttpGateway.GetMetadata().GetNamespace(),
								},
							},
						},
					},
				},
				ProxyNames:    []string{defaults.GatewayProxyName},
				BindAddress:   defaults.GatewayBindAddress,
				BindPort:      defaults2.HybridPort,
				UseProxyProto: &wrappers.BoolValue{Value: false},
			}

			glooResources.HttpGateways = gatewayv1.MatchableHttpGatewayList{matchableHttpGateway}
			glooResources.Gateways = gatewayv1.GatewayList{hybridGateway}
		})

		It("works", func() {
			// destination reachable via HttpGateway
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              helper.TestServerName,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)

			// destination reachable via HybridGateway
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              helper.TestServerName,
				Service:           gatewayProxy,
				Port:              int(hybridProxyServicePort.Port),
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)
		})

	})

	Context("matchable hybrid gateway", func() {

		var (
			hybridProxyServicePort = corev1.ServicePort{
				Name:       "hybrid-proxy",
				Port:       int32(defaults2.HybridPort),
				TargetPort: intstr.FromInt(int(defaults2.HybridPort)),
				Protocol:   "TCP",
			}
		)

		exposePortOnGwProxyService := func() {
			gwSvc, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Get(ctx, gatewayProxy, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Append servicePort if not found already
			found := false
			for _, v := range gwSvc.Spec.Ports {
				if v.Name == hybridProxyServicePort.Name || v.Port == hybridProxyServicePort.Port {
					found = true
					break
				}
			}
			if !found {
				gwSvc.Spec.Ports = append(gwSvc.Spec.Ports, hybridProxyServicePort)
			}

			_, err = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Update(ctx, gwSvc, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			exposePortOnGwProxyService()

			// Create a MatchableHttpGateway
			matchableHttpGateway := &gatewayv1.MatchableHttpGateway{
				Metadata: &core.Metadata{
					Name:      "matchable-http-gateway",
					Namespace: testHelper.InstallNamespace,
				},
				HttpGateway: &gatewayv1.HttpGateway{
					// match all virtual services
				},
			}

			// Create a HybridGateway that references that MatchableHttpGateway
			hybridGateway := &gatewayv1.Gateway{
				Metadata: &core.Metadata{
					Name:      fmt.Sprintf("%s-hybrid", defaults.GatewayProxyName),
					Namespace: testHelper.InstallNamespace,
				},
				GatewayType: &gatewayv1.Gateway_HybridGateway{
					HybridGateway: &gatewayv1.HybridGateway{
						DelegatedHttpGateways: &gatewayv1.DelegatedHttpGateway{
							SelectionType: &gatewayv1.DelegatedHttpGateway_Ref{
								Ref: &core.ResourceRef{
									Name:      matchableHttpGateway.GetMetadata().GetName(),
									Namespace: matchableHttpGateway.GetMetadata().GetNamespace(),
								},
							},
						},
					},
				},
				ProxyNames:    []string{defaults.GatewayProxyName},
				BindAddress:   defaults.GatewayBindAddress,
				BindPort:      defaults2.HybridPort,
				UseProxyProto: &wrappers.BoolValue{Value: false},
			}

			glooResources.HttpGateways = gatewayv1.MatchableHttpGatewayList{matchableHttpGateway}
			glooResources.Gateways = gatewayv1.GatewayList{hybridGateway}
		})

		It("works", func() {
			// destination reachable via HttpGateway
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              helper.TestServerName,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)

			// destination reachable via HybridGateway
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              helper.TestServerName,
				Service:           gatewayProxy,
				Port:              int(hybridProxyServicePort.Port),
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)
		})

	})

})

func getRouteTable(name string, labels map[string]string, route *gatewayv1.Route) *gatewayv1.RouteTable {
	return &gatewayv1.RouteTable{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: testHelper.InstallNamespace,
			Labels:    labels,
		},
		Routes: []*gatewayv1.Route{route},
	}
}

func getRouteWithDirectResponse(message string, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: path,
			},
		}},
		Action: &gatewayv1.Route_DirectResponseAction{
			DirectResponseAction: &gloov1.DirectResponseAction{
				Status: 200,
				Body:   message,
			},
		},
	}
}

func getRouteWithDest(dest *gloov1.Destination, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: path,
			},
		}},
		Action: &gatewayv1.Route_RouteAction{
			RouteAction: &gloov1.RouteAction{
				Destination: &gloov1.RouteAction_Single{
					Single: dest,
				},
			},
		},
	}
}

func getRouteWithDelegateRef(delegate string, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: path,
			},
		}},
		Action: &gatewayv1.Route_DelegateAction{
			DelegateAction: &gatewayv1.DelegateAction{
				DelegationType: &gatewayv1.DelegateAction_Ref{
					Ref: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      delegate,
					},
				},
			},
		},
	}
}

func petstore(namespace string) (*appsv1.Deployment, *corev1.Service) {
	deployment := fmt.Sprintf(`
##########################
#                        #
#        Example         #
#        Service         #
#                        #
#                        #
##########################
# petstore service
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: petstore
  name: petstore
  namespace: %s
spec:
  selector:
    matchLabels:
      app: petstore
  replicas: 1
  template:
    metadata:
      labels:
        app: petstore
    spec:
      containers:
      - image: soloio/petstore-example:latest
        name: petstore
        ports:
        - containerPort: 8080
          name: http
`, namespace)

	var dep appsv1.Deployment
	err := yaml.Unmarshal([]byte(deployment), &dep)
	Expect(err).NotTo(HaveOccurred())

	service := fmt.Sprintf(`
---
apiVersion: v1
kind: Service
metadata:
  name: petstore
  namespace: %s
  labels:
    service: petstore
spec:
  ports:
  - port: 8080
    protocol: TCP
  selector:
    app: petstore
`, namespace)

	var svc corev1.Service
	err = yaml.Unmarshal([]byte(service), &svc)
	Expect(err).NotTo(HaveOccurred())

	return &dep, &svc
}
