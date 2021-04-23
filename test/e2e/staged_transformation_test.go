package e2e_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/protobuf/ptypes/wrappers"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/v1helpers"
)

var _ = Describe("Staged Transformation", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		tu            *v1helpers.TestUpstream
		envoyPort     uint32
		up            *gloov1.Upstream
		proxy         *gloov1.Proxy
	)

	BeforeEach(func() {
		proxy = nil
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		envoyPort = defaults.HttpPort

		// this upstream doesn't need to exist - in fact, we want ext auth to fail.
		extauthn := &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "extauth-server",
				Namespace: "default",
			},
			UseHttp2: &wrappers.BoolValue{Value: true},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &gloov1static.UpstreamSpec{
					Hosts: []*gloov1static.Host{{
						Addr: "127.2.3.4",
						Port: 1234,
					}},
				},
			},
		}

		ref := extauthn.Metadata.Ref()
		ns := defaults.GlooSystem
		ro := &services.RunOptions{
			NsToWrite: ns,
			NsToWatch: []string{"default", ns},
			Settings: &gloov1.Settings{
				Extauth: &extauthv1.Settings{
					ExtauthzServerRef: ref,
				},
			},
			WhatToRun: services.What{
				DisableGateway: true,
				DisableUds:     true,
				DisableFds:     true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		_, err = testClients.UpstreamClient.Write(extauthn, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		err = envoyInstance.RunWithRoleAndRestXds(ns+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		up = tu.Upstream
		_, err = testClients.UpstreamClient.Write(up, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

	setProxyWithModifier := func(et *transformation.TransformationStages, modifier func(*gloov1.VirtualHost)) {
		proxy = getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, up.Metadata.Ref())
		vs := proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener.
			VirtualHosts[0]
		vs.Options = &gloov1.VirtualHostOptions{
			StagedTransformations: et,
			Extauth: &extauthv1.ExtAuthExtension{
				Spec: &extauthv1.ExtAuthExtension_Disable{
					Disable: true,
				},
			},
		}
		if modifier != nil {
			modifier(vs)
		}
		var err error
		proxy, err = testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	}
	setProxy := func(et *transformation.TransformationStages) {
		setProxyWithModifier(et, nil)
	}

	Context("no auth", func() {

		TestUpstreamReachable := func() {
			v1helpers.TestUpstreamReachable(envoyPort, tu, nil)
		}
		It("should transform response", func() {
			setProxy(&transformation.TransformationStages{
				Early: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						Matchers: []*matchers.HeaderMatcher{
							{
								Name:  ":status",
								Value: "200",
							},
						},
						ResponseTransformation: &transformation.Transformation{
							TransformationType: &transformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									BodyTransformation: &envoytransformation.TransformationTemplate_Body{
										Body: &envoytransformation.InjaTemplate{
											Text: "early-transformed",
										},
									},
								},
							},
						},
					}},
				},
				// add regular response to see that the early one overrides it
				Regular: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						Matchers: []*matchers.HeaderMatcher{
							{
								Name:  ":status",
								Value: "200",
							},
						},
						ResponseTransformation: &transformation.Transformation{
							TransformationType: &transformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									BodyTransformation: &envoytransformation.TransformationTemplate_Body{
										Body: &envoytransformation.InjaTemplate{
											Text: "regular-transformed",
										},
									},
								},
							},
						},
					}},
				},
			})
			TestUpstreamReachable()

			// send a request and expect it transformed!
			body := []byte("test")
			v1helpers.ExpectHttpOK(body, nil, envoyPort, "early-transformed")
		})

		It("should not transform when auth succeeds", func() {
			setProxy(&transformation.TransformationStages{
				Early: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						ResponseCodeDetails: "ext_authz_error",
						ResponseTransformation: &transformation.Transformation{
							TransformationType: &transformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									BodyTransformation: &envoytransformation.TransformationTemplate_Body{
										Body: &envoytransformation.InjaTemplate{
											Text: "early-transformed",
										},
									},
								},
							},
						},
					}},
				},
			})
			TestUpstreamReachable()

			// send a request and expect it transformed!
			body := []byte("test")
			v1helpers.ExpectHttpOK(body, nil, envoyPort, "test")
		})

		It("should allow multiple header values for the same header when using HeadersToAppend", func() {
			setProxy(&transformation.TransformationStages{
				Regular: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						ResponseTransformation: &transformation.Transformation{
							TransformationType: &transformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									Headers: map[string]*envoytransformation.InjaTemplate{
										"x-custom-header": {Text: "original header"},
									},
									HeadersToAppend: []*envoytransformation.TransformationTemplate_HeaderToAppend{
										{
											Key:   "x-custom-header",
											Value: &envoytransformation.InjaTemplate{Text: "{{upper(\"appended header 1\")}}"},
										},
										{
											Key:   "x-custom-header",
											Value: &envoytransformation.InjaTemplate{Text: "{{upper(\"appended header 2\")}}"},
										},
									},
								},
							},
						},
					}},
				},
			})
			TestUpstreamReachable()

			var client http.Client
			res, err := client.Post(fmt.Sprintf("http://%s:%d/1", "localhost", envoyPort), "application/octet-stream", nil)
			Expect(err).NotTo(HaveOccurred())
			fmt.Printf("%+v\n", res.Header)
			Expect(res.Header["X-Custom-Header"]).To(ContainElements("original header", "APPENDED HEADER 1", "APPENDED HEADER 2"))
		})

		It("should apply transforms from most specific level only", func() {
			vhostTransform := &transformation.TransformationStages{
				Regular: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						ResponseTransformation: &transformation.Transformation{
							TransformationType: &transformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									Headers: map[string]*envoytransformation.InjaTemplate{
										"x-solo-1": {Text: "vhost header"},
									},
								},
							},
						},
					}},
				},
			}
			setProxyWithModifier(vhostTransform, func(vhost *gloov1.VirtualHost) {
				vhost.GetRoutes()[0].Options = &gloov1.RouteOptions{
					StagedTransformations: &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											Headers: map[string]*envoytransformation.InjaTemplate{
												"x-solo-2": {Text: "route header"},
											},
										},
									},
								},
							}},
						},
					},
				}
			})
			TestUpstreamReachable()
			var response *http.Response
			Eventually(func() error {
				var err error
				response, err = http.DefaultClient.Get(fmt.Sprintf("http://localhost:%d/1", envoyPort))
				return err
			}, "30s", "1s").ShouldNot(HaveOccurred())
			// Only route level transformations should be applied here due to the nature of envoy choosing
			// the most specific config (weighted cluster > route > vhost)
			// This behaviour can be overridden (in the control plane) by using `inheritableTransformations` to merge
			// transformations down to the route level.
			Expect(response.Header.Get("x-solo-2")).To(Equal("route header"))
			Expect(response.Header.Get("x-solo-1")).To(BeEmpty())
		})
	})

	Context("with auth", func() {
		TestUpstreamReachable := func() {
			Eventually(func() error {
				_, err := http.DefaultClient.Get(fmt.Sprintf("http://localhost:%d/1", envoyPort))
				return err
			}, "30s", "1s").ShouldNot(HaveOccurred())
		}

		It("should transform response code details", func() {
			setProxyWithModifier(&transformation.TransformationStages{
				Early: &transformation.RequestResponseTransformations{
					ResponseTransforms: []*transformation.ResponseMatch{{
						ResponseCodeDetails: "ext_authz_error",
						ResponseTransformation: &transformation.Transformation{
							TransformationType: &transformation.Transformation_TransformationTemplate{
								TransformationTemplate: &envoytransformation.TransformationTemplate{
									ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
									BodyTransformation: &envoytransformation.TransformationTemplate_Body{
										Body: &envoytransformation.InjaTemplate{
											Text: "early-transformed",
										},
									},
								},
							},
						},
					}},
				},
			}, func(vs *gloov1.VirtualHost) {
				vs.Options.Extauth = &extauthv1.ExtAuthExtension{
					Spec: &extauthv1.ExtAuthExtension_CustomAuth{
						CustomAuth: &extauthv1.CustomAuth{},
					},
				}
			})
			TestUpstreamReachable()
			// send a request and expect it transformed!
			res, err := http.DefaultClient.Get(fmt.Sprintf("http://localhost:%d/1", envoyPort))
			Expect(err).NotTo(HaveOccurred())

			body, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("early-transformed"))
		})
	})

})
