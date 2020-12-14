package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	//envoy_config_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	//envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
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

var _ = FDescribe("CSRF Test", func() {

	const (
		filter_string         = "\"numerator\": 100"
		extact_matcher_string = "\"exact\": \"allowThisOne.solo.io\""
		matcher_string        = "\"ignore_case\": true"
	)

	var td csrfTestData

	apiFilter := &gloo_config_core.RuntimeFractionalPercent{
		DefaultValue: &glootype.FractionalPercent{
			Numerator:   uint32(100),
			Denominator: glootype.FractionalPercent_HUNDRED,
		},
	}

	apiAdditionalOrigins := []*gloo_type_matcher.StringMatcher{
		{
			MatchPattern: &gloo_type_matcher.StringMatcher_Exact{
				Exact: "allowThisOne.solo.io",
			},
			IgnoreCase:    true,
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

	FContext("with envoy", func() {
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

		It("should run with csrf filter", func() {
			allowedOrigins := []string{"allowThisOne.solo.io"}
			allowedMethods := []string{"GET", "POST"}
			csrf := &csrf.CsrfPolicy{
				FilterEnabled:     apiFilter,
				ShadowEnabled:     &gloo_config_core.RuntimeFractionalPercent{},
				AdditionalOrigins: apiAdditionalOrigins,
			}

			td.setupInitialProxy(csrf)
			envoyConfig := td.per.getEnvoyConfig()

			By("Check config")
			Expect(envoyConfig).To(MatchRegexp(filter_string))
			Expect(envoyConfig).To(MatchRegexp(extact_matcher_string))
			Expect(envoyConfig).To(MatchRegexp(matcher_string))

			print(envoyConfig)

			By("Request with allowed origin")
			mockOrigin := allowedOrigins[0]
			h := td.per.getOptions(mockOrigin, "GET")
			v, ok := h[requestACHMethods]
			Expect(ok).To(BeTrue())
			Expect(strings.Split(v[0], ",")).Should(ConsistOf(allowedMethods))
			v, ok = h[requestACHOrigin]
			Expect(ok).To(BeTrue())
			Expect(len(v)).To(Equal(1))
			Expect(v[0]).To(Equal(mockOrigin))
		})
	})

})

func (td *csrfTestData) getGlooCsrfProxy(csrf *csrf.CsrfPolicy) (*gloov1.Proxy, error) {
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
	}, "10s", ".1s").Should(BeNil())
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

// To test this with curl:
// curl -H "Origin: http://example.com" \
//   -H "Access-Control-Request-Method: POST" \
//   -H "Access-Control-Request-Headers: X-Requested-With" \
//   -X OPTIONS --verbose localhost:11082
func (ptd *perCsrfTestData) getOptions(origin, method string) http.Header {
	h := http.Header{}
	Eventually(func() error {
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
		h = resp.Header
		return nil
	}, "10s", ".1s").Should(BeNil())
	return h
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
