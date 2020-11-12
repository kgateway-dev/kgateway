package gloo_mtls_test

import (
	"context"
	"strconv"
	"time"

	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/go-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"k8s.io/client-go/rest"
)

var _ = Describe("Installing gloo edge in mtls mode", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
		cfg    *rest.Config

		gatewayClient        gatewayv1.GatewayClient
		virtualServiceClient gatewayv1.VirtualServiceClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		cfg, err = kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		cache := kube.NewKubeCache(ctx)
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

		virtualServiceClient, err = gatewayv1.NewVirtualServiceClient(virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())

	})

	AfterEach(func() {
		cancel()
	})

	It("can route request to upstream", func() {
		testRunnerUpstreamName := testHelper.InstallNamespace + "-" + helper.TestrunnerName + "-" + strconv.Itoa(helper.TestRunnerPort)
		vs := getTrivialVirtualServiceForUpstream(testHelper.InstallNamespace, core.ResourceRef{Name: testRunnerUpstreamName, Namespace: testHelper.InstallNamespace})
		_, err := virtualServiceClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		defaultGateway := defaults.DefaultGateway(testHelper.InstallNamespace)
		// wait for default gateway to be created
		Eventually(func() (*gatewayv1.Gateway, error) {
			return gatewayClient.Read(testHelper.InstallNamespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
		}, "15s", "0.5s").Should(Not(BeNil()))

		gatewayPort := 80
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "http",
			Path:              "/",
			Method:            "GET",
			Host:              defaults.GatewayProxyName,
			Service:           defaults.GatewayProxyName,
			Port:              gatewayPort,
			ConnectionTimeout: 10, // this is important, as the first curl call sometimes hangs indefinitely
		}, helper.SimpleHttpResponse, 1, time.Minute*20)
	})

})

func getTrivialVirtualServiceForUpstream(ns string, upstream core.ResourceRef) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: core.Metadata{
			Name:      "vs",
			Namespace: ns,
		},
		VirtualHost: &gatewayv1.VirtualHost{
			Domains: []string{"*"},
			Routes: []*gatewayv1.Route{{
				Action: &gatewayv1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{
								DestinationType: &gloov1.Destination_Upstream{
									Upstream: utils.ResourceRefPtr(upstream),
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
}
