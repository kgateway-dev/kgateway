package gateway_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/solo-io/go-utils/threadsafe"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/google/uuid"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/compress"

	"github.com/solo-io/gloo/test/ginkgo/parallel"

	"github.com/onsi/gomega/types"

	ratelimit2 "github.com/solo-io/gloo/projects/gloo/api/external/solo/ratelimit"
	v1alpha1skv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	"github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	rlv1alpha1 "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	gloo_matchers "github.com/solo-io/solo-kit/test/matchers"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/debug"
	"google.golang.org/grpc"

	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gwtranslator "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	grpcv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	gloorest "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/rest"
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

	admissionregv1 "k8s.io/api/admissionregistration/v1"
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
				}, ContainSubstring(kube2e.TestServerHttpResponse()), 1, 60*time.Second, 1*time.Second)
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

				httpEcho.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Service:           gatewayProxy,
					Port:              int(defaults2.TcpPort),
					ConnectionTimeout: 1,
				}, ContainSubstring(responseString), 0, 30*time.Second)
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

				httpEcho.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "https",
					Sni:               httpEchoClusterName,
					Service:           clusterIp,
					Port:              int(defaults2.TcpPort),
					ConnectionTimeout: 1,
					SelfSigned:        true,
				}, ContainSubstring(responseString), 0, 30*time.Second)
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

	Context("Validation Configuration", Ordered, func() {
		// These tests explicitly test the behavior of Gloo Edge when our Validation API is modified
		// Ideally all tests run with the most restrictive validation settings, so we try to isolate these tests
		// Also, adjusting the validation configuration takes time to propagate (server restart), so we write tests
		// in order, which allows us to use BeforeAll instead of BeforeEach (https://onsi.github.io/ginkgo/#setup-in-ordered-containers-beforeall-and-afterall)

		expectResourceRejected := func(yaml string, errorMatcher types.GomegaMatcher) {
			GinkgoHelper()

			var stderr threadsafe.Buffer

			_ = kubeCli.Command(ctx, "apply", "-f", "-").
				WithStdin(bytes.NewBuffer([]byte(yaml))).
				WithStderr(&stderr).
				Run()
			Expect(string(stderr.Bytes())).To(errorMatcher)
		}

		expectResourceAccepted := func(yaml string) {
			GinkgoHelper()

			err := kubeCli.Apply(ctx, []byte(yaml))
			Expect(err).NotTo(HaveOccurred())

			// To ensure that we do not leave artifacts between tests
			// we clean up the resource after it is accepted
			err = kubeCli.Delete(ctx, []byte(yaml))
			Expect(err).NotTo(HaveOccurred())
		}

		verifyGlooValidationWorks := func() {
			// Validation of Gloo resources requires that a Proxy resource exist
			// Therefore, before the tests start, we must attempt updates that should be rejected
			// They will only be rejected once a Proxy exists in the ApiSnapshot

			placeholderUs := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "",
					Namespace: testHelper.InstallNamespace,
				},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static.UpstreamSpec{
						Hosts: []*static.Host{{
							Addr: "~",
						}},
					},
				},
			}
			attempt := 0
			Eventually(func(g Gomega) bool {
				placeholderUs.Metadata.Name = fmt.Sprintf("invalid-placeholder-us-%d", attempt)

				_, err := resourceClientset.UpstreamClient().Write(placeholderUs, clients.WriteOpts{Ctx: ctx})
				if err != nil {
					serr := err.Error()
					g.Expect(serr).Should(ContainSubstring("admission webhook"))
					g.Expect(serr).Should(ContainSubstring("port cannot be empty for host"))
					// We have successfully rejected an invalid upstream
					// This means that the webhook is fully warmed, and contains a Snapshot with a Proxy
					return true
				}

				err = resourceClientset.UpstreamClient().Delete(
					placeholderUs.GetMetadata().GetNamespace(),
					placeholderUs.GetMetadata().GetName(),
					clients.DeleteOpts{Ctx: ctx, IgnoreNotExist: true})
				g.Expect(err).NotTo(HaveOccurred())

				attempt += 1
				return false
			}, time.Second*15, time.Second*1).Should(BeTrue())
		}

		When("allowWarnings=false", Ordered, func() {

			BeforeAll(func() {
				// Set the validation settings to be as strict as possible so that we can trigger
				// rejections by just producing a warning on the resource
				kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
					settings.GetGateway().GetValidation().AllowWarnings = &wrappers.BoolValue{Value: false}
				}, testHelper.InstallNamespace)
			})

			AfterAll(func() {
				kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
					settings.GetGateway().GetValidation().AllowWarnings = &wrappers.BoolValue{Value: true}
				}, testHelper.InstallNamespace)
			})

			It("Rejects invalid Gateway resources", func() {
				verifyGlooValidationWorks()

				// specifically avoiding using a DescribeTable here in order to avoid reinstalling
				// for every test case
				type testCase struct {
					resourceYaml string
					errorMatcher types.GomegaMatcher
				}

				testCases := []testCase{
					{
						resourceYaml: `
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: ` + testHelper.InstallNamespace + `
spec:
  virtualHoost: {}
`,
						// This is handled by validation schemas now
						// We support matching on number of options, in order to support our nightly tests,
						// which are run using our earliest and latest supported versions of Kubernetes
						errorMatcher: Or(
							// This is the error returned when running Kubernetes <1.25
							ContainSubstring(`ValidationError(VirtualService.spec): unknown field "virtualHoost" in io.solo.gateway.v1.VirtualService.spec`),
							// This is the error returned when running Kubernetes >= 1.25
							ContainSubstring(`Error from server (BadRequest): error when creating "STDIN": VirtualService in version "v1" cannot be handled as a VirtualService: strict decoding error: unknown field "spec.virtualHoost"`)),
					},
					{
						resourceYaml: `
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: method-matcher
  namespace: ` + testHelper.InstallNamespace + `
spec:
  virtualHost:
    domains:
     - unique2
    routes:
      - matchers:
        - exact: /delegated-nonprefix  # not allowed
        delegateAction:
          name: does-not-exist # also not allowed, but caught later
          namespace: anywhere
`,
						errorMatcher: ContainSubstring(gwtranslator.MissingPrefixErr.Error()),
					},
					{
						resourceYaml: `
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-without-type
  namespace: ` + testHelper.InstallNamespace + `
spec:
  bindAddress: '::'
`,
						errorMatcher: ContainSubstring(gwtranslator.MissingGatewayTypeErr.Error()),
					},
					{
						resourceYaml: `
apiVersion: ratelimit.solo.io/v1alpha1
kind: RateLimitConfig
metadata:
  name: rlc
  namespace: gloo-system
spec:
  raw:
    descriptors:
      - key: foo
        value: foo
        rateLimit:
          requestsPerUnit: 1
          unit: MINUTE
    rateLimits:
      - actions:
        - genericKey:
            descriptorValue: bar
`,
						errorMatcher: ContainSubstring("The Gloo Advanced Rate limit API feature 'RateLimitConfig' is enterprise-only"),
					},
				}

				for _, tc := range testCases {
					expectResourceRejected(tc.resourceYaml, tc.errorMatcher)
				}
			})

			It("Rejects invalid Gloo resources", func() {
				verifyGlooValidationWorks()

				// specifically avoiding using a DescribeTable here in order to avoid reinstalling
				// for every test case
				type testCase struct {
					resourceYaml string
					errorMatcher types.GomegaMatcher
				}

				testCases := []testCase{{
					resourceYaml: `
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: invalid-upstream
  namespace: gloo-system
spec:
  static:
    hosts:
      - addr: ~
`,
					errorMatcher: ContainSubstring("addr cannot be empty for host\n"),
				}}
				for _, tc := range testCases {
					expectResourceRejected(tc.resourceYaml, tc.errorMatcher)
				}
			})

			Context("secret validation", func() {
				const secretName = "tls-secret"

				BeforeEach(func() {
					tlsSecret := helpers.GetTlsSecret(secretName, testHelper.InstallNamespace)
					glooResources.Secrets = gloov1.SecretList{tlsSecret}

					// Modify the VirtualService to include the created SslConfig
					testServerVs.SslConfig = &ssl.SslConfig{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      tlsSecret.GetMetadata().GetName(),
								Namespace: tlsSecret.GetMetadata().GetNamespace(),
							},
						},
					}
				})

				// There are times when the VirtualService + Proxy do not update Status with the error when deleting the referenced Secret, therefore the validation error doesn't occur.
				// It isn't until later - either a few minutes and/or after forcing an update by updating the VS - that the error status appears.
				// The reason is still unknown, so we retry on flakes in the meantime.
				It("should act as expected with secret validation", func() {
					verifyGlooValidationWorks()

					By("waiting for the modified VS to be accepted")
					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, testServerVs.GetMetadata().GetName(), clients.ReadOpts{Ctx: ctx})
					})

					By("failing to delete a secret that is in use")
					err := resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Delete(ctx, secretName, metav1.DeleteOptions{})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(testmatchers.ContainSubstrings([]string{"admission webhook", "SSL secret not found", secretName}))

					By("successfully deleting a secret that is no longer in use")
					// We patch the VirtualService to remove the ssl reference, allowing the Secret to be removed
					err = helpers.PatchResource(
						ctx,
						&core.ResourceRef{
							Namespace: testHelper.InstallNamespace,
							Name:      testServerVs.GetMetadata().Name,
						},
						func(resource resources.Resource) resources.Resource {
							vs, ok := resource.(*gatewayv1.VirtualService)
							Expect(ok).To(BeTrue())
							vs.SslConfig = nil
							return vs
						},
						resourceClientset.VirtualServiceClient().BaseClient())
					Expect(err).NotTo(HaveOccurred())
					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, testServerVs.GetMetadata().GetName(), clients.ReadOpts{Ctx: ctx})
					})

					// Although these tests delete the secret handled by our SnapshotWriter, because we set `IgnoreNotFound` when deleting snapshot resources, this won't cause an issue.
					Eventually(func() error {
						return resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Delete(ctx, secretName, metav1.DeleteOptions{})
					}).WithPolling(500 * time.Millisecond).WithTimeout(5 * time.Second).ShouldNot(HaveOccurred())
				})

				It("can delete a secret that is not in use", func() {
					tlsSecret := helpers.GetKubeSecret("tls-secret-2", testHelper.InstallNamespace)
					tlsSecret, err := resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Create(ctx, tlsSecret, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())

					err = resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Delete(ctx, tlsSecret.GetName(), metav1.DeleteOptions{})
					Expect(err).NotTo(HaveOccurred())
				})
			})

		})

		When("allowWarnings=true", Ordered, func() {

			BeforeAll(func() {
				kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
					settings.GetGateway().GetValidation().AllowWarnings = &wrappers.BoolValue{Value: true}
				}, testHelper.InstallNamespace)
			})

			AfterAll(func() {
				// Our tests default to using allowWarnings=true, so we just need to ensure we leave it that way
			})

			It("Accepts valid Gateway resources", func() {
				// specifically avoiding using a DescribeTable here in order to avoid reinstalling
				// for every test case
				type testCase struct {
					resourceYaml string
				}

				testCases := []testCase{
					{
						resourceYaml: `
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: missing-upstream
  namespace: ` + testHelper.InstallNamespace + `
spec:
  virtualHost:
    domains:
     - unique1
    routes:
      - matchers:
        - methods:
           - GET
          prefix: /items/
        routeAction:
          single:
            upstream:
              name: does-not-exist
              namespace: anywhere
`,
					},
				}

				for _, tc := range testCases {
					expectResourceAccepted(tc.resourceYaml)
				}
			})

		})

		// These are the conditions to check secret deletion functionality/validation against current errors with allowWarnings=false and there are warnings
		When("allowWarnings=false, FailurePolicy=Fail and there are warnings", Ordered, func() {
			const (
				secretName       = "tls-secret"
				unusedSecretName = "tls-secret-unused"
			)

			var (
				invalidUpstreamYaml      string
				vsYaml                   string
				pretestFailurePolicyType admissionregv1.FailurePolicyType
			)

			// Before these secret deletion tests, set the failure policy to Fail and setup the resources with warnings
			BeforeAll(func() {
				invalidUpstreamYaml = `
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: my-us
  namespace: ` + testHelper.InstallNamespace + `
spec:
  kube:
    serviceName: my-svc
    serviceNamespace: ` + testHelper.InstallNamespace + `
    servicePort: 18081
    serviceSpec:
      grpc: {}`

				vsYaml = `
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: my-vs
spec:
   virtualHost:
    domains:
    - valid.local
    options:
    routes:
    - matchers:
        - prefix: /
      routeAction:
        single:
          upstream:
            name: my-us
            namespace:  ` + testHelper.InstallNamespace

				// Store the current failure policy to restore after the tests
				pretestFailurePolicyType = *kube2e.GetFailurePolicy(ctx, "gloo-gateway-validation-webhook-"+testHelper.InstallNamespace)

				kube2e.UpdateFailurePolicy(ctx, "gloo-gateway-validation-webhook-"+testHelper.InstallNamespace, admissionregv1.Fail)
				// Allow warnings during setup so that we can install the resources
				kube2e.UpdateAllowWarningsSetting(ctx, true, testHelper.InstallNamespace)

				// This should work regardless of whether the warnings are allowed or not, as an invalid upstream is not a warning until it part of a route
				err := kubeCli.Apply(ctx, []byte(invalidUpstreamYaml))
				Expect(err).NotTo(HaveOccurred())

				// Use an "Eventually" here, as this is the step that actually causes the warnings, so the changes need to have been propagated
				Eventually(func(g Gomega) {
					err := kubeCli.Apply(ctx, []byte(vsYaml))
					g.Expect(err).NotTo(HaveOccurred())
				}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			})

			AfterAll(func() {
				kube2e.UpdateFailurePolicy(ctx, "gloo-gateway-validation-webhook-"+testHelper.InstallNamespace, pretestFailurePolicyType)
				err := kubeCli.Delete(ctx, []byte(invalidUpstreamYaml))
				Expect(err).NotTo(HaveOccurred())
				err = kubeCli.Delete(ctx, []byte(vsYaml))
				Expect(err).NotTo(HaveOccurred())

				// Our tests default to using allowWarnings=true, so we just need to ensure we leave it that way
				kube2e.UpdateAllowWarningsSetting(ctx, true, testHelper.InstallNamespace)
			})

			// The outer "JustBeforeEach" writes the snapshot, and it will have warnings, so in BeforeEach, allowWarnings is set to true
			BeforeEach(func() {
				// This call is probably redundant becaise of the BeforeAll/AfterEach calls that do the same thing,
				// but it to protects against any changes in the future
				kube2e.UpdateAllowWarningsSetting(ctx, true, testHelper.InstallNamespace)

				tlsSecret := helpers.GetTlsSecret(secretName, testHelper.InstallNamespace)
				glooResources.Secrets = gloov1.SecretList{tlsSecret}

				tlsSecretUnused := helpers.GetTlsSecret(unusedSecretName, testHelper.InstallNamespace)
				glooResources.Secrets = append(glooResources.Secrets, tlsSecretUnused)

				// Modify the VirtualService to include the created SslConfig
				testServerVs.SslConfig = &ssl.SslConfig{
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: &core.ResourceRef{
							Name:      tlsSecret.GetMetadata().GetName(),
							Namespace: tlsSecret.GetMetadata().GetNamespace(),
						},
					},
				}

			})

			AfterEach(func() {
				kube2e.UpdateAllowWarningsSetting(ctx, true, testHelper.InstallNamespace)

				// Ensure the secrets are deleted after the test. Ignore the errors, as they may have been deleted already
				resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Delete(ctx, secretName, metav1.DeleteOptions{})
				resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Delete(ctx, unusedSecretName, metav1.DeleteOptions{})

				// Check that they're gone
				_, err := resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Get(ctx, secretName, metav1.GetOptions{})
				Expect(err).To(MatchError("secrets \"" + secretName + "\" not found"))
				_, err = resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Get(ctx, unusedSecretName, metav1.GetOptions{})
				Expect(err).To(MatchError("secrets \"" + unusedSecretName + "\" not found"))
			})

			// After the outer "JustBeforeEach" writes the snapshot, set allowWarnings to false for the tests
			JustBeforeEach(func() {
				kube2e.UpdateAllowWarningsSetting(ctx, false, testHelper.InstallNamespace)
			})

			It("should act as expected with secret validation", FlakeAttempts(3), func() {
				By("waiting for the modified VS to be accepted")
				helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					return resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, testServerVs.GetMetadata().GetName(), clients.ReadOpts{Ctx: ctx})
				})

				By("Rejecting resource patches due to existing warnings") // Make sure `allowWarnings` is being respected
				err := helpers.PatchResource(
					ctx,
					&core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      testServerVs.GetMetadata().Name,
					},
					func(resource resources.Resource) resources.Resource {
						vs, ok := resource.(*gatewayv1.VirtualService)
						Expect(ok).To(BeTrue())
						vs.SslConfig = nil
						return vs
					},
					resourceClientset.VirtualServiceClient().BaseClient())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(testmatchers.ContainSubstrings([]string{"references the service", "which does not exist in namespace"}))

				By("failing to delete a secret that is in use")
				err = resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Delete(ctx, secretName, metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(testmatchers.ContainSubstrings([]string{"admission webhook", "SSL secret not found", secretName}))

				// No test for removing a secret from use and deleting it, as the modification to remove the secret from the route would not be allowed due to allowWarnings=false
				By("deleting a secret that is not in use")
				err = resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Delete(ctx, unusedSecretName, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

			})

		})

		Context("DisableTransformationValidation", func() {

			var (
				invalidTransformation *glootransformation.Transformations
			)

			BeforeEach(func() {
				validInjaTransform := `{% if default(data.error.message, "") != "" %}400{% else %}{{ header(":status") }}{% endif %}`
				invalidTransformation = &glootransformation.Transformations{
					ClearRouteCache: true,
					ResponseTransformation: &glootransformation.Transformation{
						TransformationType: &glootransformation.Transformation_TransformationTemplate{
							TransformationTemplate: &glootransformation.TransformationTemplate{
								Headers: map[string]*glootransformation.InjaTemplate{
									":status": {Text: strings.TrimSuffix(validInjaTransform, "}")},
								},
							},
						},
					},
				}
			})

			It("rejects invalid inja template in transformation", func() {
				// update the test server vs
				err := helpers.PatchResource(
					ctx,
					&core.ResourceRef{
						Namespace: testServerVs.GetMetadata().GetNamespace(),
						Name:      testServerVs.GetMetadata().GetName(),
					},
					func(resource resources.Resource) resources.Resource {
						vs := resource.(*gatewayv1.VirtualService)
						vs.VirtualHost.Options = &gloov1.VirtualHostOptions{Transformations: invalidTransformation}
						return vs
					},
					resourceClientset.VirtualServiceClient().BaseClient(),
				)
				Expect(err).To(MatchError(ContainSubstring("Failed to parse response template: Failed to parse " +
					"header template ':status': [inja.exception.parser_error] (at 1:92) expected statement close, got '%'")))
			})

			Context("Extractors", func() {
				var (
					// each test will define and then write this extraction to the test server vs
					extraction *glootransformation.Extraction

					// helper used in patchVirtualServiceWithExtraction
					createTransformationFromExtraction = func(extraction *glootransformation.Extraction) *glootransformation.Transformations {
						return &glootransformation.Transformations{
							ClearRouteCache: true,
							ResponseTransformation: &glootransformation.Transformation{
								TransformationType: &glootransformation.Transformation_TransformationTemplate{
									TransformationTemplate: &glootransformation.TransformationTemplate{
										Headers: map[string]*glootransformation.InjaTemplate{
											":path": {Text: "/"},
										},
										Extractors: map[string]*glootransformation.Extraction{
											"foo": extraction,
										},
									},
								},
							},
						}
					}

					// helper used in each test to patch the test server vs to include the extraction
					patchVirtualServiceWithExtraction = func(ctx context.Context, namespace, name string, extraction *glootransformation.Extraction) error {
						return helpers.PatchResource(
							ctx,
							&core.ResourceRef{
								Namespace: namespace,
								Name:      name,
							},
							func(resource resources.Resource) resources.Resource {
								vs := resource.(*gatewayv1.VirtualService)
								vs.VirtualHost.Options = &gloov1.VirtualHostOptions{
									Transformations: createTransformationFromExtraction(extraction),
								}
								return vs
							},
							resourceClientset.VirtualServiceClient().BaseClient(),
						)
					}
				)

				It("Extract mode -- rejects invalid subgroup in transformation", func() {
					extraction = &glootransformation.Extraction{
						Source: &glootransformation.Extraction_Header{
							Header: ":path",
						},
						// note that the regex has no subgroups, but we are trying to extract the first subgroup
						// this should be rejected
						Regex:    ".*",
						Subgroup: 1,
						Mode:     glootransformation.Extraction_EXTRACT,
					}
					// update the test server vs
					err := patchVirtualServiceWithExtraction(ctx, testServerVs.GetMetadata().GetNamespace(), testServerVs.GetMetadata().GetName(), extraction)
					Expect(err).To(MatchError(ContainSubstring("envoy validation mode output: error initializing configuration '': Failed to parse response template: group 1 requested for regex with only 0 sub groups")))
				})

				It("Single replace mode -- rejects invalid subgroup in transformation", func() {
					extraction = &glootransformation.Extraction{
						Source: &glootransformation.Extraction_Header{
							Header: ":path",
						},
						// note that the regex has no subgroups, but we are trying to extract the first subgroup
						// this should be rejected
						Regex:           ".*",
						Subgroup:        1,
						ReplacementText: &wrappers.StringValue{Value: "bar"},
						Mode:            glootransformation.Extraction_SINGLE_REPLACE,
					}
					// update the test server vs
					err := patchVirtualServiceWithExtraction(ctx, testServerVs.GetMetadata().GetNamespace(), testServerVs.GetMetadata().GetName(), extraction)
					Expect(err).To(MatchError(ContainSubstring("envoy validation mode output: error initializing configuration '': Failed to parse response template: group 1 requested for regex with only 0 sub groups")))
				})
			})

			Context("disable_transformation_validation is set", Ordered, func() {

				BeforeAll(func() {
					kube2e.UpdateDisableTransformationValidationSetting(ctx, true, testHelper.InstallNamespace)
				})

				AfterAll(func() {
					kube2e.UpdateDisableTransformationValidationSetting(ctx, false, testHelper.InstallNamespace)
				})

				It("will not reject invalid transformation", func() {
					err := helpers.PatchResource(
						ctx,
						&core.ResourceRef{
							Namespace: testServerVs.GetMetadata().GetNamespace(),
							Name:      testServerVs.GetMetadata().GetName(),
						},
						func(resource resources.Resource) resources.Resource {
							vs := resource.(*gatewayv1.VirtualService)
							vs.VirtualHost.Options = &gloov1.VirtualHostOptions{Transformations: invalidTransformation}
							return vs
						},
						resourceClientset.VirtualServiceClient().BaseClient(),
					)
					Expect(err).NotTo(HaveOccurred())
					Eventually(func(g Gomega) {
						vs, err := resourceClientset.VirtualServiceClient().Read(
							testServerVs.GetMetadata().GetNamespace(),
							testServerVs.GetMetadata().GetName(),
							clients.ReadOpts{Ctx: ctx})
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(vs.GetVirtualHost().GetOptions().GetTransformations()).To(gloo_matchers.MatchProto(invalidTransformation))
					}).Should(Succeed())

				})
			})

		})

		When("alwaysAccept=true", Ordered, func() {
			// We want to test behaviors when Gloo allows invalid resouces to be persisted

			var uniqueSuffix int

			BeforeAll(func() {
				uniqueSuffix = 0
				kube2e.UpdateAlwaysAcceptSetting(ctx, true, testHelper.InstallNamespace)
			})

			AfterAll(func() {
				// important that we update the always accept setting after removing resources, or else we can have:
				// "validation is disabled due to an invalid resource which has been written to storage.
				// Please correct any Rejected resources to re-enable validation."
				kube2e.UpdateAlwaysAcceptSetting(ctx, false, testHelper.InstallNamespace)
			})

			Context("with a mix of valid and invalid virtual services", FlakeAttempts(3), func() {
				// We have resolved most of the flakiness in this test, but it still occasionally fails
				// We have not been able to reproduce the failure locally, so we are marking it as flaky

				var (
					validVsName, invalidVsName string
				)

				BeforeEach(func() {
					// For each test, uniquely identify the resources using a suffix that is unique per test
					// We quickly apply and delete the same values, so we want to ensure a new snapshot (according to Gloo) is created
					uniqueSuffix += 1
					validVsName = fmt.Sprintf("i-am-valid-%d-%d", uniqueSuffix, parallel.GetParallelProcessCount())
					invalidVsName = fmt.Sprintf("i-am-invalid-%d-%d", uniqueSuffix, parallel.GetParallelProcessCount())

					validVs := helpers.NewVirtualServiceBuilder().
						WithName(validVsName).
						WithNamespace(testHelper.InstallNamespace).
						WithDomain("valid1.com").
						WithRoutePrefixMatcher("route", "/").
						WithRouteActionToUpstreamRef("route",
							&core.ResourceRef{
								Name:      kubernetesplugin.UpstreamName(testHelper.InstallNamespace, helper.TestServerName, helper.TestServerPort),
								Namespace: testHelper.InstallNamespace,
							}).
						Build()

					invalidVs := helpers.NewVirtualServiceBuilder().
						WithName(invalidVsName).
						WithNamespace(testHelper.InstallNamespace).
						WithDomain("invalid.com").
						WithRouteMatcher("route", &matchers.Matcher{}).
						WithRouteOptions("route",
							&gloov1.RouteOptions{
								PrefixRewrite: &wrappers.StringValue{Value: "matcher and action are missing"},
							}).
						Build()

					glooResources.VirtualServices = []*gatewayv1.VirtualService{validVs, invalidVs}
				})

				JustBeforeEach(func() {
					// ensure that we have successfully gotten into an invalid state
					helpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
						return resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, invalidVsName, clients.ReadOpts{
							Ctx: ctx,
						})
					})
				})

				It("propagates the valid virtual services to envoy", func() {
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/",
						Method:            "GET",
						Host:              "valid1.com",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/",
						Method:            "GET",
						Host:              "invalid.com",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
						Verbose:           true,
					}, ContainSubstring(`HTTP/1.1 404 Not Found`), 1, 60*time.Second, 1*time.Second)
				})

				It("preserves the valid virtual services in envoy when a virtual service has been made invalid", func() {
					invalidVs, err := resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, invalidVsName, clients.ReadOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())

					validVs, err := resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, validVsName, clients.ReadOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())

					// make the invalid vs valid and the valid vs invalid
					invalidVh := invalidVs.VirtualHost
					validVh := validVs.VirtualHost
					validVh.Domains = []string{"all-good-in-the-hood.com"}

					invalidVs.VirtualHost = validVh
					validVs.VirtualHost = invalidVh
					statusClient := gloostatusutils.GetStatusClientForNamespace(testHelper.InstallNamespace)
					virtualServiceReconciler := gatewayv1.NewVirtualServiceReconciler(resourceClientset.VirtualServiceClient(), statusClient)
					err = virtualServiceReconciler.Reconcile(testHelper.InstallNamespace, gatewayv1.VirtualServiceList{validVs, invalidVs}, nil, clients.ListOpts{})
					Expect(err).NotTo(HaveOccurred())

					// the original virtual service should work
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/",
						Method:            "GET",
						Host:              "valid1.com",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)

					// the fixed virtual service should also work
					testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
						Protocol:          "http",
						Path:              "/",
						Method:            "GET",
						Host:              "all-good-in-the-hood.com",
						Service:           gatewayProxy,
						Port:              gatewayPort,
						ConnectionTimeout: 1,
						WithoutStats:      true,
					}, kube2e.TestServerHttpResponse(), 1, 60*time.Second, 1*time.Second)
				})

				Context("adds the invalid virtual services back into the proxy", func() {

					var (
						err                  error
						petstoreName         = "petstore"
						petstoreUpstreamName string
						petstoreSvc          *corev1.Service
						petstoreDeployment   *appsv1.Deployment
					)

					BeforeEach(func() {
						petstoreUpstreamName = kubernetesplugin.UpstreamName(testHelper.InstallNamespace, petstoreName, 8080)
						petstoreDeployment, petstoreSvc = petstore(testHelper.InstallNamespace)

						// disable FDS for the petstore, create it without functions
						petstoreSvc.Labels[syncer.FdsLabelKey] = "disabled"

						petstoreSvc, err = resourceClientset.KubeClients().CoreV1().Services(petstoreSvc.Namespace).Create(ctx, petstoreSvc, metav1.CreateOptions{})
						Expect(err).NotTo(HaveOccurred())
						petstoreDeployment, err = resourceClientset.KubeClients().AppsV1().Deployments(petstoreDeployment.Namespace).Create(ctx, petstoreDeployment, metav1.CreateOptions{})
						Expect(err).NotTo(HaveOccurred())

						petstoreVs := helpers.NewVirtualServiceBuilder().
							WithName(petstoreName).
							WithNamespace(testHelper.InstallNamespace).
							WithDomain("petstore.com").
							WithRoutePrefixMatcher(petstoreName, "/").
							WithRouteActionToSingleDestination(petstoreName,
								&gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: &core.ResourceRef{
											Namespace: testHelper.InstallNamespace,
											Name:      petstoreUpstreamName,
										},
									},
									DestinationSpec: &gloov1.DestinationSpec{
										DestinationType: &gloov1.DestinationSpec_Rest{
											Rest: &gloorest.DestinationSpec{
												FunctionName: "findPetById",
											},
										},
									},
								}).
							Build()

						glooResources.VirtualServices = append(glooResources.VirtualServices, []*gatewayv1.VirtualService{
							petstoreVs,
						}...)
					})

					JustBeforeEach(func() {
						// The Upstream should be created by discovery
						helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
							return resourceClientset.UpstreamClient().Read(testHelper.InstallNamespace, petstoreUpstreamName, clients.ReadOpts{})
						})

						// the VS should not be rejected since the failure is sanitized by route replacement
						helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
							return resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, petstoreName, clients.ReadOpts{})
						})
					})

					AfterEach(func() {
						_ = resourceClientset.KubeClients().CoreV1().Services(petstoreSvc.Namespace).Delete(ctx, petstoreName, metav1.DeleteOptions{})
						helpers.EventuallyObjectDeleted(func() (client.Object, error) {
							return resourceClientset.KubeClients().CoreV1().Services(petstoreSvc.Namespace).Get(ctx, petstoreName, metav1.GetOptions{})
						})

						_ = resourceClientset.KubeClients().AppsV1().Deployments(petstoreDeployment.Namespace).Delete(ctx, petstoreName, metav1.DeleteOptions{})
						helpers.EventuallyObjectDeleted(func() (client.Object, error) {
							return resourceClientset.KubeClients().AppsV1().Deployments(petstoreDeployment.Namespace).Get(ctx, petstoreName, metav1.GetOptions{})
						})
					})

					It("when updating an upstream makes them valid", func() {
						err = helpers.PatchResource(
							ctx,
							&core.ResourceRef{
								Namespace: testHelper.InstallNamespace,
								Name:      petstoreUpstreamName,
							},
							func(resource resources.Resource) resources.Resource {
								us := resource.(*gloov1.Upstream)
								us.Metadata.Labels[syncer.FdsLabelKey] = "enabled"
								return us
							},
							resourceClientset.UpstreamClient().BaseClient(),
						)
						Expect(err).NotTo(HaveOccurred())

						// FDS should update the upstream with discovered rest spec
						// it can take a long time for this to happen, perhaps petstore wasn't healthy yet?
						Eventually(func() interface{} {
							petstoreUs, err := resourceClientset.UpstreamClient().Read(testHelper.InstallNamespace, petstoreUpstreamName, clients.ReadOpts{Ctx: ctx})
							Expect(err).ToNot(HaveOccurred())
							return petstoreUs.GetKube().GetServiceSpec().GetRest().GetSwaggerInfo().GetUrl()
						}, "120s", "1s").ShouldNot(BeEmpty())

						// we have updated an upstream, which prompts Gloo to send a notification to the
						// gateway to resync virtual service status

						// the VS should get accepted
						helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
							return resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, petstoreName, clients.ReadOpts{Ctx: ctx})
						})
					})

				})

			})

			Context("RateLimitConfigs", func() {

				var rateLimitConfig *v1alpha1skv1.RateLimitConfig

				BeforeEach(func() {
					rateLimitConfig = &v1alpha1skv1.RateLimitConfig{
						RateLimitConfig: ratelimit2.RateLimitConfig{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testrlc",
								Namespace: testHelper.InstallNamespace,
							},
							Spec: rlv1alpha1.RateLimitConfigSpec{
								ConfigType: &rlv1alpha1.RateLimitConfigSpec_Raw_{
									Raw: &rlv1alpha1.RateLimitConfigSpec_Raw{
										Descriptors: []*rlv1alpha1.Descriptor{{
											Key:   "generic_key",
											Value: "foo",
											RateLimit: &rlv1alpha1.RateLimit{
												Unit:            rlv1alpha1.RateLimit_MINUTE,
												RequestsPerUnit: 1,
											},
										}},
										RateLimits: []*rlv1alpha1.RateLimitActions{{
											Actions: []*rlv1alpha1.Action{{
												ActionSpecifier: &rlv1alpha1.Action_GenericKey_{
													GenericKey: &rlv1alpha1.Action_GenericKey{
														DescriptorValue: "foo",
													},
												},
											}},
										}},
									},
								},
							},
						},
					}
					glooResources.Ratelimitconfigs = v1alpha1skv1.RateLimitConfigList{rateLimitConfig}
				})

				It("correctly sets a status to a RateLimitConfig", func() {
					// demand that a created ratelimit config _has_ a rejected status.
					Eventually(func(g Gomega) error {
						rlc, err := resourceClientset.RateLimitConfigClient().Read(rateLimitConfig.GetMetadata().GetNamespace(), rateLimitConfig.GetMetadata().GetName(), clients.ReadOpts{Ctx: ctx})
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(rlc.Status.State).To(Equal(v1alpha1.RateLimitConfigStatus_REJECTED))
						g.Expect(rlc.Status.Message).Should(ContainSubstring("enterprise-only"))
						return nil
					}, "15s", "0.5s").ShouldNot(HaveOccurred())
				})
			})
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
