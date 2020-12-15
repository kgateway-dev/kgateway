package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"errors"
	"github.com/onsi/gomega"
	gloo_config_core "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	gloo_type_matcher "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	glootype "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	"io"
	"net/http"

	csrf "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type perCsrfTestData struct {
	up            *gloov1.Upstream
	envoyInstance *services.EnvoyInstance
	envoyPort     uint32
	envoyAdminUrl string
}
type csrfTestData struct {
	testClients services.TestClients
	ctx         context.Context
	cancel      context.CancelFunc
	per         perCsrfTestData
}

var _ = FDescribe("csrf 2", func() {

	var td csrfTestData

	apiFilter := &gloo_config_core.RuntimeFractionalPercent{
		DefaultValue: &glootype.FractionalPercent{
			Numerator:   uint32(100),
			Denominator: glootype.FractionalPercent_HUNDRED,
		},
	}

	apiAdditionalOrigins := []*gloo_type_matcher.StringMatcher{
		{
			MatchPattern: &gloo_type_matcher.StringMatcher_SafeRegex{
				SafeRegex: &gloo_type_matcher.RegexMatcher{
					EngineType: &gloo_type_matcher.RegexMatcher_GoogleRe2{
						GoogleRe2: &gloo_type_matcher.RegexMatcher_GoogleRE2{},
					},
					Regex: "allowThisOne.solo.io",
				},
			},
		},
	}

	BeforeEach(func() {
		td.ctx, td.cancel = context.WithCancel(context.Background())
		td.testClients = services.RunGateway(td.ctx, true)
		td.per = perCsrfTestData{}
	})

	AfterEach(func() {
		td.cancel()
	})

	Context("with envoy", func() {

		BeforeEach(func() {
			var err error
			td.per.envoyInstance, err = envoyFactory.NewEnvoyInstance()
			Expect(err).NotTo(HaveOccurred())
			td.per.envoyAdminUrl = fmt.Sprintf("http://%s:%d/config_dump",
				td.per.envoyInstance.LocalAddr(),
				td.per.envoyInstance.AdminPort)

			err = td.per.envoyInstance.Run(td.testClients.GlooPort)
			Expect(err).NotTo(HaveOccurred())

			td.per.up = td.setupUpstream()
		})

		AfterEach(func() {
			if td.per.envoyInstance != nil {
				td.per.envoyInstance.Clean()
			}
		})

		It("should run with csrf", func() {
			//allowedOrigins := []string{"allowThisOne.solo.io"}

			csrf := &csrf.CsrfPolicy{
				FilterEnabled:     apiFilter,
				AdditionalOrigins: apiAdditionalOrigins,
			}

			td.setupInitialProxy(csrf)

			//By("Request with allowed origin")
			//mockOrigin := allowedOrigins[0]
			//td.per.testRequest(mockOrigin, "GET").Should(BeNil())

			By("Request with unallowed origin")
			mockOriginUnallowed := "notAllowed"
			td.per.testRequest(mockOriginUnallowed, "GET").ShouldNot(BeNil())
		})
	})
})

func (td *csrfTestData) getGlooCsrfProxy(csrf * csrf.CsrfPolicy) (*gloov1.Proxy, error) {
	readProxy, err := td.testClients.ProxyClient.Read("default", "proxy", clients.ReadOpts{})
	if err != nil {
		return nil, err
	}
	return td.per.getGlooCsrfProxyWithVersion(readProxy.Metadata.ResourceVersion, csrf), nil
}

func (ptd *perCsrfTestData) getGlooCsrfProxyWithVersion(resourceVersion string, csrf *csrf.CsrfPolicy) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:            "proxy",
			Namespace:       "default",
			ResourceVersion: resourceVersion,
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "0.0.0.0",
			BindPort:    ptd.envoyPort,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: []*gloov1.VirtualHost{{
						Name:    "virt1",
						Domains: []string{"*"},
						Routes: []*gloov1.Route{{
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: ptd.up.Metadata.Ref(),
											},
										},
									},
								},
							},
						}},
						Options: &gloov1.VirtualHostOptions{
							Csrf: csrf,
						},
					}},
				},
			},
		}},
	}
}

func (td *csrfTestData) setupProxy(proxy *gloov1.Proxy) error {
	proxyCli := td.testClients.ProxyClient
	_, err := proxyCli.Write(proxy, clients.WriteOpts{OverwriteExisting: true})
	return err
}

func (td *csrfTestData) setupInitialProxy(csrf *csrf.CsrfPolicy) {
	By("Setup proxy")
	td.per.envoyPort = defaults.HttpPort
	proxy := td.per.getGlooCsrfProxyWithVersion("", csrf)
	err := td.setupProxy(proxy)
	// Call with retries to ensure proxy is available
	Eventually(func() error {
		proxy, err := td.getGlooCsrfProxy(csrf)
		if err != nil {
			return err
		}
		return td.setupProxy(proxy)
	}, "10s", ".1s").Should(BeNil())
	Expect(err).NotTo(HaveOccurred())
	Eventually(func() error {
		_, err := http.Get(fmt.Sprintf("http://%s:%d/status/200", "localhost", td.per.envoyPort))
		if err != nil {
			return err
		}
		return nil
	}, "1m", ".1s").Should(BeNil())
}

func (td *csrfTestData) setupUpstream() *gloov1.Upstream {
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

func (ptd *perCsrfTestData) testRequest(origin, method string) gomega.AsyncAssertion {
	return Eventually(func() error {
		req, err := http.NewRequest("OPTIONS", fmt.Sprintf("http://localhost:%v", ptd.envoyPort), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", method)
		req.Header.Set("Access-Control-Request-Headers", "X-Requested-With")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			return nil
		}
		print(resp.StatusCode)
		return errors.New(fmt.Sprintf("status code: %d", resp.StatusCode))
	}, "20s", ".1s")
}

func (ptd *perCsrfTestData) getEnvoyConfig() string {
	By("Get config")
	envoyConfig := ""
	Eventually(func() error {
		r, err := http.Get(ptd.envoyAdminUrl)
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
	return envoyConfig
}
