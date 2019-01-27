package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Transformations", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		t := services.RunGateway(ctx, true)
		testClients = t
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.Run(testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

	})

	AfterEach(func() {
		if envoyInstance != nil {
			envoyInstance.Clean()
		}
		cancel()
	})

	It("should should transform json to html response", func() {

		tu := v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		var opts clients.WriteOpts
		up := tu.Upstream
		_, err := testClients.UpstreamClient.Write(up, opts)
		Expect(err).NotTo(HaveOccurred())

		proxycli := testClients.ProxyClient
		envoyPort := services.NextBindPort()
		proxy := &gloov1.Proxy{
			Metadata: core.Metadata{
				Name:      "proxy",
				Namespace: "default",
			},
			Listeners: []*gloov1.Listener{{
				Name:        "listener",
				BindAddress: "127.0.0.1",
				BindPort:    envoyPort,
				ListenerType: &gloov1.Listener_HttpListener{
					HttpListener: &gloov1.HttpListener{
						VirtualHosts: []*gloov1.VirtualHost{{
							Name:    "virt1",
							Domains: []string{"*"},
							Routes: []*gloov1.Route{{
								RoutePlugins: &gloov1.RoutePlugins{
									Transformations: &envoy_transform.RouteTransformations{
										ResponseTransformation: &envoy_transform.Transformation{
											TransformationType: &envoy_transform.Transformation_TransformationTemplate{
												TransformationTemplate: &envoy_transform.TransformationTemplate{
													BodyTransformation: &envoy_transform.TransformationTemplate_Body{
														Body: &envoy_transform.InjaTemplate{
															Text: "{{body}}",
														},
													},
													Headers: map[string]*envoy_transform.InjaTemplate{
														"content-type": {
															Text: "text/html",
														},
													},
												},
											},
										},
									},
								},
								Matcher: &gloov1.Matcher{
									PathSpecifier: &gloov1.Matcher_Prefix{
										Prefix: "/",
									},
								},
								Action: &gloov1.Route_RouteAction{
									RouteAction: &gloov1.RouteAction{
										Destination: &gloov1.RouteAction_Single{
											Single: &gloov1.Destination{
												Upstream: up.Metadata.Ref(),
											},
										},
									},
								},
							}},
						}},
					},
				},
			}},
		}

		_, err = proxycli.Write(proxy, opts)
		Expect(err).NotTo(HaveOccurred())

		body := []byte("{\"body\":\"test\"}")

		Eventually(func() (string, error) {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)

			res, err := http.Post(fmt.Sprintf("http://%s:%d/1", "localhost", envoyPort), "application/octet-stream", &buf)
			if err != nil {
				return "", err
			}
			if res.StatusCode != http.StatusOK {
				return "", errors.New(fmt.Sprintf("%v is not OK", res.StatusCode))
			}
			b, err := ioutil.ReadAll(res.Body)
			return string(b), err
		}, "5s", ".5s").Should(Equal("test"))

	})
})
