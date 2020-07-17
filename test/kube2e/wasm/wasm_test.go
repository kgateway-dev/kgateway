package gateway_test

import (
	"context"
	"fmt"
	"time"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/wasm"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/go-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/gogo/protobuf/types"
	"k8s.io/client-go/rest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Kube2e: wasm", func() {

	const (
		gatewayProxy = defaults.GatewayProxyName
		gatewayPort  = int(80)
	)

	var (
		ctx    context.Context
		cancel context.CancelFunc
		cfg    *rest.Config
		cache  kube.SharedCache

		gatewayClient        gatewayv1.GatewayClient
		virtualServiceClient gatewayv1.VirtualServiceClient
		routeTableClient     gatewayv1.RouteTableClient
		upstreamGroupClient  gloov1.UpstreamGroupClient
		upstreamClient       gloov1.UpstreamClient
		proxyClient          gloov1.ProxyClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		cache = kube.NewKubeCache(ctx)
		gatewayClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.GatewayCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		virtualServiceClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.VirtualServiceCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		routeTableClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gatewayv1.RouteTableCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		upstreamGroupClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.UpstreamGroupCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		upstreamClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.UpstreamCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		proxyClientFactory := &factory.KubeResourceClientFactory{
			Crd:         gloov1.ProxyCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		gatewayClient, err = gatewayv1.NewGatewayClient(gatewayClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = gatewayClient.Register()
		Expect(err).NotTo(HaveOccurred())

		virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualServiceClient.Register()
		Expect(err).NotTo(HaveOccurred())

		routeTableClient, err = gatewayv1.NewRouteTableClient(routeTableClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = routeTableClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamGroupClient, err = gloov1.NewUpstreamGroupClient(upstreamGroupClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamGroupClient.Register()
		Expect(err).NotTo(HaveOccurred())

		upstreamClient, err = gloov1.NewUpstreamClient(upstreamClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = upstreamClient.Register()
		Expect(err).NotTo(HaveOccurred())

		proxyClient, err = gloov1.NewProxyClient(proxyClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = proxyClient.Register()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("tests with virtual service", func() {

		AfterEach(func() {
			cancel()
			err := virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{IgnoreNotExist: true})
			Expect(err).NotTo(HaveOccurred())
		})

		It("correctly routes requests to an upstream", func() {
			dest := &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      fmt.Sprintf("%s-%s-%v", testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort),
					},
				},
			}
			// give proxy validation a chance to start
			Eventually(func() error {
				_, err := virtualServiceClient.Write(getVirtualService(dest, nil), clients.WriteOpts{})
				return err
			}).ShouldNot(HaveOccurred())

			defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
			// wait for default gateway to be created
			Eventually(func() (*gatewayv1.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))

			gateway, err := gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			gw, ok := gateway.GetGatewayType().(*gatewayv1.Gateway_HttpGateway)
			Expect(ok).To(BeTrue())

			configVal := types.StringValue{Value: "test"}
			configAny, err := types.MarshalAny(&configVal)

			gw.HttpGateway.Options = &gloov1.HttpListenerOptions{
				Wasm: &wasm.PluginSource{
					Filters: []*wasm.WasmFilter{{
						Image:  "webassemblyhub.io/sodman/example-filter:v0.2",
						Config: configAny,
						Name:   "add-header-wasm-test",
						RootId: "add_header_root_id",
					}},
				},
			}

			writtenGW, err := gatewayClient.Write(gateway, clients.WriteOpts{
				OverwriteExisting: true,
			})
			Expect(err).NotTo(HaveOccurred())

			fmt.Printf("gatewayWritten without errors - %v\n", writtenGW)

			// wait for default gateway to be updated
			Eventually(func() (*gatewayv1.Gateway, error) {
				return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			}, "15s", "0.5s").Should(Not(BeNil()))
			wasmHeader := "valuefromconfig: test"

			co := helper.CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Verbose:           true,
				Port:              gatewayPort,
				ConnectionTimeout: 1, // this is important, as sometimes curl hangs
				WithoutStats:      true,
			}

			newGateway, err := gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			fmt.Printf("NEW GATEWAY YO - %v\n", newGateway)
			// Should still have a successful response
			testHelper.CurlEventuallyShouldRespond(co, helper.SimpleHttpResponse, 1, 60*time.Second, 1*time.Second)

			// Check for the header added by the wasm filter
			testHelper.CurlEventuallyShouldOutput(co, wasmHeader, 1, 60*time.Second, 1*time.Second)
		})

	})

})

func getVirtualService(dest *gloov1.Destination, sslConfig *gloov1.SslConfig) *gatewayv1.VirtualService {
	return getVirtualServiceWithRoute(getRouteWithDest(dest, "/"), sslConfig)
}

func getVirtualServiceWithRoute(route *gatewayv1.Route, sslConfig *gloov1.SslConfig) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: core.Metadata{
			Name:      "vs",
			Namespace: testHelper.InstallNamespace,
		},
		SslConfig: sslConfig,
		VirtualHost: &gatewayv1.VirtualHost{
			Domains: []string{"*"},

			Routes: []*gatewayv1.Route{route},
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
