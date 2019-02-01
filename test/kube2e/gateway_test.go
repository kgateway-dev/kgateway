package kube2e_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/test/setup"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Kube2e: gateway", func() {
	It("works", func() {
		cfg, err := kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())
		cache := kube.NewKubeCache(context.TODO())
		gatewayClientFactory := &factory.KubeResourceClientFactory{
			Crd:         v1.GatewayCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}
		virtualServiceClientFactory := &factory.KubeResourceClientFactory{
			Crd:         v1.VirtualServiceCrd,
			Cfg:         cfg,
			SharedCache: cache,
		}

		gatewayClient, err := v1.NewGatewayClient(gatewayClientFactory)
		Expect(err).NotTo(HaveOccurred())

		virtualServiceClient, err := v1.NewVirtualServiceClient(virtualServiceClientFactory)
		Expect(err).NotTo(HaveOccurred())

		_, err = virtualServiceClient.Write(&v1.VirtualService{

			Metadata: core.Metadata{
				Name:      "vs",
				Namespace: namespace,
			},
			VirtualHost: &gloov1.VirtualHost{
				Name:    "default",
				Domains: []string{"*"},
				Routes: []*gloov1.Route{{
					Matcher: &gloov1.Matcher{
						PathSpecifier: &gloov1.Matcher_Prefix{
							Prefix: "/",
						},
					},
					Action: &gloov1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									Upstream: core.ResourceRef{Namespace: namespace, Name: fmt.Sprintf("%s-%s-%v", namespace, "testrunner", testRunnerPort)},
								},
							},
						},
					},
				}},
			},
		}, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		defaultGateway := defaults.DefaultGateway(namespace)
		// wait for default gateway to be created
		Eventually(func() (*v1.Gateway, error) {
			return gatewayClient.Read(namespace, defaultGateway.Metadata.Name, clients.ReadOpts{})
		}, "5s", "0.5s").Should(Not(BeNil()))

		gatewayProxy := "gateway-proxy"
		gatewayPort := int(gloodefaults.HttpPort)
		setup.CurlEventuallyShouldRespond(setup.CurlOpts{
			Protocol: "http",
			Path:     "/",
			Method:   "GET",
			Host:     gatewayProxy,
			Service:  gatewayProxy,
			Port:     gatewayPort,
		}, helpers.SimpleHttpResponse, time.Minute)
	})
})
