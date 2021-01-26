package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	transformation "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Transformations", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		envoyPort     uint32
		tu            *v1helpers.TestUpstream
		opts          clients.WriteOpts
		transform     *transformation.Transformations
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		ns := defaults.GlooSystem
		ro := &services.RunOptions{
			NsToWrite: ns,
			NsToWatch: []string{"default", ns},
			WhatToRun: services.What{
				DisableGateway: true,
				DisableFds:     true,
				DisableUds:     true,
			},
		}

		testClients = services.RunGlooGatewayUdsFds(ctx, ro)
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRoleAndRestXds(services.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())
		envoyPort = defaults.HttpPort

		tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		opts = clients.WriteOpts{
			Ctx: ctx,
		}
		_, err = testClients.UpstreamClient.Write(tu.Upstream, opts)
		Expect(err).NotTo(HaveOccurred())
		transform = &transformation.Transformations{
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
		}
	})

	AfterEach(func() {
		if envoyInstance != nil {
			envoyInstance.Clean()
		}
		cancel()
	})

	ExpectSuccess := func() {

		body := []byte("{\"body\":\"test\"}")

		client := &http.Client{Timeout: time.Second}

		Eventually(func() (string, error) {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)

			res, err := client.Post(fmt.Sprintf("http://%s:%d/1", "localhost", envoyPort), "application/octet-stream", &buf)
			if err != nil {
				return "", err
			}
			if res.StatusCode != http.StatusOK {
				return "", errors.New(fmt.Sprintf("%v is not OK", res.StatusCode))
			}
			b, err := ioutil.ReadAll(res.Body)
			return string(b), err
		}, "20s", ".5s").Should(Equal("test"))
	}

	WriteVhost := func(vs *gloov1.VirtualHost) {
		proxycli := testClients.ProxyClient
		proxy := &gloov1.Proxy{
			Metadata: &core.Metadata{
				Name:      "proxy",
				Namespace: "default",
			},
			Listeners: []*gloov1.Listener{{
				Name:        "listener",
				BindAddress: "0.0.0.0",
				BindPort:    envoyPort,
				ListenerType: &gloov1.Listener_HttpListener{
					HttpListener: &gloov1.HttpListener{
						VirtualHosts: []*gloov1.VirtualHost{vs},
					},
				},
			}},
		}

		_, err := proxycli.Write(proxy, opts)
		Expect(err).NotTo(HaveOccurred())
	}

	It("should should transform json to html response on vhost", func() {
		WriteVhost(&gloov1.VirtualHost{
			Options: &gloov1.VirtualHostOptions{
				Transformations: transform,
			},
			Name:    "virt1",
			Domains: []string{"*"},
			Routes: []*gloov1.Route{{
				Action: &gloov1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{
								DestinationType: &gloov1.Destination_Upstream{
									Upstream: tu.Upstream.Metadata.Ref(),
								},
							},
						},
					},
				},
			}},
		})

		ExpectSuccess()
	})

	It("should should transform json to html response on route", func() {
		WriteVhost(&gloov1.VirtualHost{
			Name:    "virt1",
			Domains: []string{"*"},
			Routes: []*gloov1.Route{{
				Options: &gloov1.RouteOptions{
					Transformations: transform,
				},
				Action: &gloov1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{
								DestinationType: &gloov1.Destination_Upstream{
									Upstream: tu.Upstream.Metadata.Ref(),
								},
							},
						},
					},
				},
			}},
		})

		ExpectSuccess()
	})

	It("should should transform json to html response on route", func() {
		WriteVhost(&gloov1.VirtualHost{
			Name:    "virt1",
			Domains: []string{"*"},
			Routes: []*gloov1.Route{{
				Action: &gloov1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Multi{
							Multi: &gloov1.MultiDestination{
								Destinations: []*gloov1.WeightedDestination{
									{
										Weight: 1,
										Options: &gloov1.WeightedDestinationOptions{
											Transformations: transform,
										},
										Destination: &gloov1.Destination{

											DestinationType: &gloov1.Destination_Upstream{
												Upstream: tu.Upstream.Metadata.Ref(),
											},
										},
									},
								},
							},
						},
					},
				},
			}},
		})

		ExpectSuccess()
	})

})
