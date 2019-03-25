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

type perCorsTestData struct {
	up            *gloov1.Upstream
	envoyInstance *services.EnvoyInstance
	envoyPort     uint32
	envoyAdminUrl string
}
type corsTestData struct {
	testClients services.TestClients
	ctx         context.Context
	per         perCorsTestData
}

var _ = Describe("CORS", func() {

	var td corsTestData

	BeforeEach(func() {
		td.ctx, _ = context.WithCancel(context.Background())
		td.testClients = services.RunGateway(td.ctx, true)
		td.per = perCorsTestData{}
	})

	Context("with envoy", func() {

		BeforeEach(func() {
			var err error
			td.per.envoyInstance, err = envoyFactory.NewEnvoyInstance()
			Expect(err).NotTo(HaveOccurred())
			td.per.envoyAdminUrl = fmt.Sprintf("http://%s:%d/config_dump", "localhost", td.per.envoyInstance.AdminPort)

			err = td.per.envoyInstance.Run(td.testClients.GlooPort)
			Expect(err).NotTo(HaveOccurred())

			td.per.up = td.setupUpstream()
		})

		AfterEach(func() {
			if td.per.envoyInstance != nil {
				td.per.envoyInstance.Clean()
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
			By("Setup initial proxy")
			td.setupInitialProxy(cors)
			By("Set cors")
			Eventually(func() error {
				proxy, err := td.getGlooCorsProxy(cors)
				if err != nil {
					return err
				}
				return td.setupProxy(proxy)
			}, "10s", ".1s").Should(BeNil())

			envoyConfig := ""
			By("Get config")
			Eventually(func() error {
				r, err := http.Get(td.per.envoyAdminUrl)
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

func (td *corsTestData) getGlooCorsProxy(cors *gloov1.CorsPolicy) (*gloov1.Proxy, error) {
	readProxy, err := td.testClients.ProxyClient.Read("default", "proxy", clients.ReadOpts{})
	if err != nil {
		return nil, err
	}
	return td.per.getGlooCorsProxyWithVersion(td.per.up, readProxy.Metadata.ResourceVersion, cors), nil
}

func (ptd *perCorsTestData) getGlooCorsProxyWithVersion(up *gloov1.Upstream, resourceVersion string, cors *gloov1.CorsPolicy) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: core.Metadata{
			Name:            "proxy",
			Namespace:       "default",
			ResourceVersion: resourceVersion,
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "127.0.0.1",
			BindPort:    ptd.envoyPort,
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

func (td *corsTestData) setupProxy(proxy *gloov1.Proxy) error {
	proxyCli := td.testClients.ProxyClient
	_, err := proxyCli.Write(proxy, clients.WriteOpts{OverwriteExisting: true})
	return err
}

func (td *corsTestData) setupInitialProxy(activeCors *gloov1.CorsPolicy) {
	td.per.envoyPort = services.NextBindPort()
	proxy := td.per.getGlooCorsProxyWithVersion(td.per.up, "", activeCors)
	err := td.setupProxy(proxy)
	Expect(err).NotTo(HaveOccurred())
	Eventually(func() error {
		_, err := http.Get(fmt.Sprintf("http://%s:%d/status/200", "localhost", td.per.envoyPort))
		if err != nil {
			return err
		}
		return nil
	}, "10s", ".1s").Should(BeNil())
}

func (td *corsTestData) setupUpstream() *gloov1.Upstream {
	tu := v1helpers.NewTestHttpUpstream(td.ctx, td.per.envoyInstance.LocalAddr())
	// drain channel as we don't care about it
	go func() {
		for range tu.C {
		}
	}()
	up := tu.Upstream
	_, err := td.testClients.UpstreamClient.Write(up, clients.WriteOpts{OverwriteExisting: true})
	Expect(err).NotTo(HaveOccurred())
	return up
}
