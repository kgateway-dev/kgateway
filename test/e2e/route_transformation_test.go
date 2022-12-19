package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
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
			ResponseTransformation: &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
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
		envoyInstance.Clean()

		cancel()
	})

	ExpectSuccess := func() {

		body := []byte("{\"body\":\"test\"}")

		client := &http.Client{Timeout: time.Second}

		EventuallyWithOffset(1, func() (string, error) {
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
			b, err := io.ReadAll(res.Body)
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
				BindAddress: net.IPv4zero.String(),
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
										Weight: &wrappers.UInt32Value{Value: 1},
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

	GetTrivialVirtualHostWithUpstreamRef := func(usRef *core.ResourceRef) *gloov1.VirtualHost {
		return &gloov1.VirtualHost{
			Name:    "virt1",
			Domains: []string{"*"},
			Routes: []*gloov1.Route{{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/",
					},
				}},
				Action: &gloov1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{
								DestinationType: &gloov1.Destination_Upstream{
									Upstream: usRef,
								},
							},
						},
					},
				},
			}},
		}
	}

	GetHttpbinEchoUpstream := func() *gloov1.Upstream {
		return &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "httpbin",
				Namespace: "default",
			},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &gloov1static.UpstreamSpec{
					Hosts: []*gloov1static.Host{
						{
							Addr: "httpbin.org",
							Port: 80,
						},
					},
				},
			},
		}
	}

	writeUpstream := func(us *gloov1.Upstream) {
		// write us
		uscli := testClients.UpstreamClient
		_, err := uscli.Write(us, opts)
		Expect(err).NotTo(HaveOccurred())

		// wait for us to be created
		Eventually(func() (*gloov1.Upstream, error) {
			return uscli.Read(us.Metadata.Namespace, us.Metadata.Name, clients.ReadOpts{})
		}, "10s", "0.5s").ShouldNot(BeNil())
	}

	FormRequestWithUrlAndHeaders := func(url string, headers map[string]string) *http.Request {
		// form request
		req, err := http.NewRequest("GET", url, nil)
		Expect(err).NotTo(HaveOccurred())
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req
	}

	GetSuccessfulResponse := func(req *http.Request) *http.Response {
		client := &http.Client{Timeout: time.Second}
		var (
			res *http.Response
			err error
		)

		Eventually(func(g Gomega) {
			// send request
			res, err = client.Do(req)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res.StatusCode).To(Equal(http.StatusOK))
		}, "10s", "1s").Should(Succeed())

		return res
	}

	ExpectUnsuccessfulResponse := func(req *http.Request) {
		client := &http.Client{Timeout: time.Second}
		var (
			res *http.Response
			err error
		)

		Eventually(func(g Gomega) {
			// send request
			res, err = client.Do(req)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
		}, "10s", "1s").Should(Succeed())
	}

	GetHtmlRequest := func() *http.Request {
		// note that the Httpbin html endpoint returns a non-json body
		url := fmt.Sprintf("http://%s:%d/html", "localhost", envoyPort)
		headers := map[string]string{
			"x-solo-hdr-1": "test",
		}
		req := FormRequestWithUrlAndHeaders(url, headers)
		return req
	}

	Context("parsing non-valid JSON", func() {
		var (
			us        *gloov1.Upstream
			vh        *gloov1.VirtualHost
			transform *transformation.Transformations
		)
		BeforeEach(func() {
			// create upstream that will return an html body at the /html endpoint
			us = GetHttpbinEchoUpstream()
			writeUpstream(us)

			// create a virtual host with a route to the upstream
			vh = GetTrivialVirtualHostWithUpstreamRef(us.Metadata.Ref())

			// add a transformation to the virtual host
			transform = &transformation.Transformations{
				ResponseTransformation: &transformation.Transformation{
					TransformationType: &transformation.Transformation_TransformationTemplate{
						TransformationTemplate: &envoy_transform.TransformationTemplate{
							Headers: map[string]*envoy_transform.InjaTemplate{
								"x-solo-resp-hdr1": {
									Text: "{{ request_header(\"x-solo-hdr-1\") }}",
								},
							},
						},
					},
				},
			}

			vh.Options = &gloov1.VirtualHostOptions{
				Transformations: transform,
			}
		})
		It("should error on non-json body when ignoreErrorOnParse/parseBodyBehavior/passthrough is disabled", func() {
			WriteVhost(vh)

			// execute request -- expect a 400 response
			ExpectUnsuccessfulResponse(GetHtmlRequest())
		})
		It("should transform response with non-json body when ignoreErrorOnParse is enabled", func() {
			transform.ResponseTransformation.GetTransformationTemplate().IgnoreErrorOnParse = true
			WriteVhost(vh)

			// execute request -- expect a 200 response
			res := GetSuccessfulResponse(GetHtmlRequest())

			// inspect response headers to confirm transformation was applied
			Expect(res.Header.Get("x-solo-resp-hdr1")).To(Equal("test"))
			// attempt to read body as json to confirm that it was not parsed
			var body map[string]interface{}
			err := json.NewDecoder(res.Body).Decode(&body)
			Expect(err).To(HaveOccurred())
		})
		It("should transform response with non-json body when ParseBodyBehavior is set to DontParse", func() {
			transform.ResponseTransformation.GetTransformationTemplate().ParseBodyBehavior = envoy_transform.TransformationTemplate_DontParse
			WriteVhost(vh)

			// execute request -- expect a 200 response
			res := GetSuccessfulResponse(GetHtmlRequest())

			// inspect response headers to confirm transformation was applied
			Expect(res.Header.Get("x-solo-resp-hdr1")).To(Equal("test"))
			// attempt to read body as json to confirm that it was not parsed
			var body map[string]interface{}
			err := json.NewDecoder(res.Body).Decode(&body)
			Expect(err).To(HaveOccurred())
		})
		It("should transform response with non-json body when passthrough is enabled", func() {
			transform.ResponseTransformation.GetTransformationTemplate().BodyTransformation = &envoy_transform.TransformationTemplate_Passthrough{
				Passthrough: &envoy_transform.Passthrough{},
			}
			WriteVhost(vh)

			// execute request -- expect a 200 response
			res := GetSuccessfulResponse(GetHtmlRequest())

			// inspect response headers to confirm transformation was applied
			Expect(res.Header.Get("x-solo-resp-hdr1")).To(Equal("test"))
			// attempt to read body as json to confirm that it was not parsed
			var body map[string]interface{}
			err := json.NewDecoder(res.Body).Decode(&body)
			Expect(err).To(HaveOccurred())
		})
	})
})
