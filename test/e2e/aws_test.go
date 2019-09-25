package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/solo-io/gloo/pkg/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/gloo/test/services"

	gw1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	aws_plugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws"

	"github.com/aws/aws-sdk-go/aws/credentials"
)

var _ = Describe("AWS Lambda", func() {
	const region = "us-east-1"

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		secret        *gloov1.Secret
		upstream      *gloov1.Upstream
	)

	addCredentials := func() {

		localAwsCredentials := credentials.NewSharedCredentials("", "")
		v, err := localAwsCredentials.Get()
		if err != nil {
			Skip("no AWS creds available")
		}
		var opts clients.WriteOpts

		accesskey := v.AccessKeyID
		secretkey := v.SecretAccessKey

		secret = &gloov1.Secret{
			Metadata: core.Metadata{
				Namespace: "default",
				Name:      region,
			},
			Kind: &gloov1.Secret_Aws{
				Aws: &gloov1.AwsSecret{
					AccessKey: accesskey,
					SecretKey: secretkey,
				},
			},
		}

		_, err = testClients.SecretClient.Write(secret, opts)
		Expect(err).NotTo(HaveOccurred())
	}

	addUpstream := func() {
		upstream = &gloov1.Upstream{
			Metadata: core.Metadata{
				Namespace: "default",
				Name:      region,
			},
			UpstreamSpec: &gloov1.UpstreamSpec{
				UpstreamType: &gloov1.UpstreamSpec_Aws{
					Aws: &aws_plugin.UpstreamSpec{
						LambdaFunctions: []*aws_plugin.LambdaFunctionSpec{{
							LambdaFunctionName: "uppercase",
							Qualifier:          "",
							LogicalName:        "uppercase",
						},
							{
								LambdaFunctionName: "contact-form",
								Qualifier:          "",
								LogicalName:        "contact-form",
							},
						},
						Region:    region,
						SecretRef: utils.ResourceRefPtr(secret.Metadata.Ref()),
					},
				},
			},
		}

		var opts clients.WriteOpts
		_, err := testClients.UpstreamClient.Write(upstream, opts)
		Expect(err).NotTo(HaveOccurred())

	}

	validateLambda := func(envoyPort uint32, substring string) {

		body := []byte("\"solo.io\"")

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

			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				return "", err
			}

			return string(body), nil
		}, "10s", "1s").Should(ContainSubstring(substring))
	}
	validateLambdaUppercase := func(envoyPort uint32) {
		validateLambda(envoyPort, "SOLO.IO")
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		testClients = services.RunGateway(ctx, false)

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		addCredentials()
		addUpstream()
	})

	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

	It("be able to call lambda", func() {
		err := envoyInstance.Run(testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		proxy := &gloov1.Proxy{
			Metadata: core.Metadata{
				Name:      "proxy",
				Namespace: "default",
			},
			Listeners: []*gloov1.Listener{{
				Name:        "listener",
				BindAddress: "::",
				BindPort:    defaults.HttpPort,
				ListenerType: &gloov1.Listener_HttpListener{
					HttpListener: &gloov1.HttpListener{
						VirtualHosts: []*gloov1.VirtualHost{{
							Name:    "virt1",
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
												DestinationType: &gloov1.Destination_Upstream{
													Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
												},
												DestinationSpec: &gloov1.DestinationSpec{
													DestinationType: &gloov1.DestinationSpec_Aws{
														Aws: &aws_plugin.DestinationSpec{
															LogicalName: "uppercase",
														},
													},
												},
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

		var opts clients.WriteOpts
		_, err = testClients.ProxyClient.Write(proxy, opts)
		Expect(err).NotTo(HaveOccurred())

		validateLambdaUppercase(defaults.HttpPort)
	})

	It("be able lambda with response transform", func() {
		err := envoyInstance.Run(testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		proxy := &gloov1.Proxy{
			Metadata: core.Metadata{
				Name:      "proxy",
				Namespace: "default",
			},
			Listeners: []*gloov1.Listener{{
				Name:        "listener",
				BindAddress: "::",
				BindPort:    defaults.HttpPort,
				ListenerType: &gloov1.Listener_HttpListener{
					HttpListener: &gloov1.HttpListener{
						VirtualHosts: []*gloov1.VirtualHost{{
							Name:    "virt1",
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
												DestinationType: &gloov1.Destination_Upstream{
													Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
												},
												DestinationSpec: &gloov1.DestinationSpec{
													DestinationType: &gloov1.DestinationSpec_Aws{
														Aws: &aws_plugin.DestinationSpec{
															LogicalName:            "contact-form",
															ResponseTransformation: true,
														},
													},
												},
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

		var opts clients.WriteOpts
		_, err = testClients.ProxyClient.Write(proxy, opts)
		Expect(err).NotTo(HaveOccurred())

		validateLambda(defaults.HttpPort, `<meta http-equiv="Content-Type" content="text/html; charset=UTF-8"/>`)
	})

	It("be able to call lambda via gateway", func() {
		err := envoyInstance.RunWithRole("gloo-system~gateway-proxy-v2", testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		vs := &gw1.VirtualService{
			Metadata: core.Metadata{
				Name:      "app",
				Namespace: "default",
			},
			VirtualHost: &gw1.VirtualHost{
				Domains: []string{"*"},
				Routes: []*gw1.Route{{
					Matcher: &gloov1.Matcher{
						PathSpecifier: &gloov1.Matcher_Prefix{
							Prefix: "/",
						},
					},
					Action: &gw1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
									},
									DestinationSpec: &gloov1.DestinationSpec{
										DestinationType: &gloov1.DestinationSpec_Aws{
											Aws: &aws_plugin.DestinationSpec{
												LogicalName: "uppercase",
											},
										},
									},
								},
							},
						},
					},
				}},
			},
		}

		var opts clients.WriteOpts
		_, err = testClients.VirtualServiceClient.Write(vs, opts)
		Expect(err).NotTo(HaveOccurred())

		validateLambdaUppercase(defaults.HttpPort)
	})
})
