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
	"github.com/rotisserie/eris"
	"k8s.io/client-go/rest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

		gatewayClient, err = gatewayv1.NewGatewayClient(gatewayClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = gatewayClient.Register()
		Expect(err).NotTo(HaveOccurred())

		virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())
		err = virtualServiceClient.Register()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("tests with virtual service", func() {

		AfterEach(func() {
			cancel()
			err := virtualServiceClient.Delete(testHelper.InstallNamespace, "vs", clients.DeleteOpts{IgnoreNotExist: true})
			Expect(err).NotTo(HaveOccurred())
		})

		It("can run a wasm filter", func() {
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

			defaultGatewayName := defaults.DefaultGateway(testHelper.InstallNamespace).Metadata.Name
			// wait for default gateway to be created
			Eventually(func() error {
				_, err := gatewayClient.Read(testHelper.InstallNamespace, defaultGatewayName, clients.ReadOpts{})
				return err
			}, "15s", "0.5s").Should(Not(HaveOccurred()))

			Eventually(func() error {
				_, err := writeWasmFilterToGateway(gatewayClient, defaultGatewayName, "dummy content to hydrate cache")
				return err
			}, "10s", "0.5s").Should(Not(HaveOccurred()), "should update gateway with wasm config")

			// There's currently an envoy bug where wasm filters not in envoy's cache will never load,
			// so we write the filter again with a new hash (changed config string) so that envoy can
			// pick it up from the cache the 2nd time.
			time.Sleep(3 * time.Second)
			Eventually(func() error {
				_, err := writeWasmFilterToGateway(gatewayClient, defaultGatewayName, "test")
				return err
			}, "10s", "0.5s").Should(Not(HaveOccurred()), "should update gateway to use cached filter")

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

// Reads the gateway from the gatewayClient using the gatewayName, and writes a basic add-header
// wasm filter config to it with the given filterName.
func writeWasmFilterToGateway(gatewayClient gatewayv1.GatewayClient, gatewayName, config string) (*gatewayv1.Gateway, error) {
	gateway, err := gatewayClient.Read(testHelper.InstallNamespace, gatewayName, clients.ReadOpts{})
	if err != nil {
		return nil, err
	}

	gw, ok := gateway.GetGatewayType().(*gatewayv1.Gateway_HttpGateway)
	if !ok {
		return nil, eris.Errorf("Listener did not have an httpGateway")
	}
	configVal := types.StringValue{Value: config}
	configAny, err := types.MarshalAny(&configVal)

	gw.HttpGateway.Options = &gloov1.HttpListenerOptions{
		Wasm: &wasm.PluginSource{
			Filters: []*wasm.WasmFilter{{
				Image:  "webassemblyhub.io/sodman/example-filter:v0.2",
				Config: configAny,
				Name:   "wasm-test-filter",
				RootId: "add_header_root_id",
			}},
		},
	}

	writtenGateway, err := gatewayClient.Write(gateway, clients.WriteOpts{
		OverwriteExisting: true,
	})
	if err != nil {
		return nil, err
	}
	return writtenGateway, nil
}
