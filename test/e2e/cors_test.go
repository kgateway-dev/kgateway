package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("CORS", func() {

	var (
		testClients services.TestClients
		ctx         context.Context
	)

	BeforeEach(func() {
		ctx, _ = context.WithCancel(context.Background())
		t := services.RunGateway(ctx, true)
		testClients = t
	})

	Context("with envoy", func() {

		var (
			envoyInstance *services.EnvoyInstance
			up            *gloov1.Upstream
			opts          clients.WriteOpts

			envoyPort     uint32
			activeCors    *gloov1.CorsPolicy
			envoyAdminUrl string
		)

		setupProxy := func(proxy *gloov1.Proxy) error {
			proxyCli := testClients.ProxyClient
			p, err := proxyCli.Write(proxy, opts)
			fmt.Println("made proxy:")
			fmt.Println(p)
			return err
		}

		setupInitialProxy := func() {
			envoyPort = services.NextBindPort()
			proxy := getGlooCorsProxyWithVersion(envoyPort, up, "", activeCors)
			err := setupProxy(proxy)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				_, err := http.Get(fmt.Sprintf("http://%s:%d/status/200", "localhost", envoyPort))
				if err != nil {
					return err
				}
				return nil
			}, "10s", ".1s").Should(BeNil())
		}

		setupUpstream := func() {
			tu := v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
			// drain channel as we dont care about it
			go func() {
				for range tu.C {
				}
			}()
			var opts clients.WriteOpts
			up = tu.Upstream
			fmt.Println("up")
			fmt.Println(up)
			_, err := testClients.UpstreamClient.Write(up, opts)
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			var err error
			envoyInstance, err = envoyFactory.NewEnvoyInstance()
			Expect(err).NotTo(HaveOccurred())
			envoyAdminUrl = fmt.Sprintf("http://%s:%d/config_dump", "localhost", envoyInstance.AdminPort)

			err = envoyInstance.Run(testClients.GlooPort)
			Expect(err).NotTo(HaveOccurred())

			setupUpstream()
		})

		AfterEach(func() {
			if envoyInstance != nil {
				envoyInstance.Clean()
			}
		})

		It("should run with cors", func() {

			allowedOrigin := "allowThisOne.solo.io"
			cors := &gloov1.CorsPolicy{
				AllowOrigin:      []string{allowedOrigin},
				AllowOriginRegex: nil,
				AllowMethods:     nil,
				AllowHeaders:     nil,
				ExposeHeaders:    nil,
				MaxAge:           "",
				AllowCredentials: false,
			}
			activeCors = cors
			setupInitialProxy()
			Eventually(func() error {
				proxy, err := getGlooCorsProxy(testClients, envoyPort, up, cors)
				if err != nil {
					return err
				}
				opts.OverwriteExisting = true
				return setupProxy(proxy)
			}, "10s", ".1s").Should(BeNil())

			envoyConfig := ""
			Eventually(func() error {
				r, err := http.Get(envoyAdminUrl)
				if err != nil {
					return err
				}
				p := new(bytes.Buffer)
				if _, err := io.Copy(p, r.Body); err != nil {
					return err
				}
				defer r.Body.Close()
				envoyConfig = p.String()
				return nil
			}, "10s", ".1s").Should(BeNil())

			Expect(envoyConfig).To(MatchRegexp("cors"))
			Expect(envoyConfig).To(MatchRegexp(allowedOrigin))

		})
	})
})

func getGlooCorsProxy(testClients services.TestClients, envoyPort uint32, up *gloov1.Upstream, cors *gloov1.CorsPolicy) (*gloov1.Proxy, error) {
	readProxy, err := testClients.ProxyClient.Read("default", "proxy", clients.ReadOpts{})
	if err != nil {
		return nil, err
	}
	return getGlooCorsProxyWithVersion(envoyPort, up, readProxy.Metadata.ResourceVersion, cors), nil
}

func getGlooCorsProxyWithVersion(envoyPort uint32, up *gloov1.Upstream, resourceVersion string, cors *gloov1.CorsPolicy) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: core.Metadata{
			Name:            "proxy",
			Namespace:       "default",
			ResourceVersion: resourceVersion,
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
						VirtualHostPlugins: &gloov1.VirtualHostPlugins{},
						CorsPolicy:         cors,
					}},
				},
			},
		}},
	}
}
