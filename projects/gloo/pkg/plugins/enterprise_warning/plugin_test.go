package enterprise_warning_test

import (
	envoy_config_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/proxylatency"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/dlp"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extproc"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/jwt"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/rbac"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/waf"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/advanced_http"
	awsapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/wasm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/enterprise_warning"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("enterprise_warning plugin", func() {

	ExpectEnterpriseOnlyErr := func(err error) {
		ExpectWithOffset(1, err).To(HaveOccurred())
		ExpectWithOffset(1, err.Error()).To(ContainSubstring("Could not load configuration for the following Enterprise features"))
	}

	Context("advanced_http", func() {

		var (
			upstream     *v1.Upstream
			upstreamSpec *v1static.UpstreamSpec
		)

		It("should not process upstream if advanced_http config is nil", func() {
			p := NewPlugin()
			err := p.ProcessUpstream(plugins.Params{}, &v1.Upstream{}, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("will err on ProcessUpstream() if advanced_http is configured", func() {
			p := NewPlugin()
			upstreamSpec = &v1static.UpstreamSpec{
				Hosts: []*v1static.Host{{
					Addr: "localhost",
					Port: 1234,
					HealthCheckConfig: &v1static.Host_HealthCheckConfig{
						Path: "/foo",
					},
				}},
			}
			upstream = &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      "extauth-server",
					Namespace: "default",
				},
				UpstreamType: &v1.Upstream_Static{
					Static: upstreamSpec,
				},
			}

			err := p.ProcessUpstream(plugins.Params{}, upstream, nil)
			ExpectEnterpriseOnlyErr(err)

			upstreamSpec.Hosts[0].HealthCheckConfig.Path = ""
			upstreamSpec.Hosts[0].HealthCheckConfig.Method = "POST"

			err = p.ProcessUpstream(plugins.Params{}, upstream, nil)
			ExpectEnterpriseOnlyErr(err)

			upstreamSpec.Hosts[0].HealthCheckConfig.Path = ""
			upstreamSpec.Hosts[0].HealthCheckConfig.Method = ""
			upstream.HealthChecks = []*core1.HealthCheck{
				{
					HealthChecker: &core1.HealthCheck_HttpHealthCheck_{
						HttpHealthCheck: &core1.HealthCheck_HttpHealthCheck{
							ResponseAssertions: &advanced_http.ResponseAssertions{},
						},
					},
				},
			}

			err = p.ProcessUpstream(plugins.Params{}, upstream, nil)
			ExpectEnterpriseOnlyErr(err)
		})

	})

	Context("dlp", func() {

		It("should not add filter if dlp config is nil", func() {
			p := NewPlugin()
			f, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeNil())
		})

		It("will err if dlp is configured", func() {
			p := NewPlugin()
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					Dlp: &dlp.FilterConfig{},
				},
			}

			f, err := p.HttpFilters(plugins.Params{}, hl)
			ExpectEnterpriseOnlyErr(err)
			Expect(f).To(BeNil())
		})

	})

	Context("failover", func() {

		It("should not process upstream if failover config is nil", func() {
			p := NewPlugin()
			err := p.ProcessUpstream(plugins.Params{}, &v1.Upstream{}, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("will err if failover is configured on process upstream", func() {
			p := NewPlugin()
			err := p.ProcessUpstream(plugins.Params{}, &v1.Upstream{Failover: &v1.Failover{}}, nil)
			ExpectEnterpriseOnlyErr(err)
		})

	})

	Context("jwt", func() {

		It("should not add filter if jwt config is nil", func() {
			p := NewPlugin()
			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{}, &envoy_config_route.VirtualHost{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("will err if jwt is configured", func() {
			p := NewPlugin()
			virtualHost := &v1.VirtualHost{
				Name:    "virt1",
				Domains: []string{"*"},
				Options: &v1.VirtualHostOptions{
					JwtConfig: &v1.VirtualHostOptions_JwtStaged{
						JwtStaged: &jwt.JwtStagedVhostExtension{},
					},
				},
			}

			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, virtualHost, &envoy_config_route.VirtualHost{})
			ExpectEnterpriseOnlyErr(err)
		})

		It("will err if jwt is configured", func() {
			p := NewPlugin()
			route := &v1.Route{
				Name: "route1",
				Options: &v1.RouteOptions{
					JwtConfig: &v1.RouteOptions_JwtStaged{
						JwtStaged: &jwt.JwtStagedRouteExtension{},
					},
				},
			}

			err := p.ProcessRoute(plugins.RouteParams{}, route, &envoy_config_route.Route{})
			ExpectEnterpriseOnlyErr(err)
		})

	})

	Context("leftmost_xff_address", func() {

		It("should not add filter if leftmost xff header config is nil", func() {
			p := NewPlugin()
			f, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeNil())
		})

		It("will err if leftmost xff header is configured", func() {
			p := NewPlugin()
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					LeftmostXffAddress: &wrappers.BoolValue{},
				},
			}

			_, err := p.HttpFilters(plugins.Params{}, hl)
			ExpectEnterpriseOnlyErr(err)
		})

	})

	Context("proxy_latency", func() {

		It("should not add filter if proxylatency config is nil", func() {
			p := NewPlugin()
			f, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeNil())
		})

		It("will err if proxylatency is configured", func() {
			p := NewPlugin()
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					ProxyLatency: &proxylatency.ProxyLatency{},
				},
			}

			f, err := p.HttpFilters(plugins.Params{}, hl)
			Expect(err).To(HaveOccurred())
			ExpectEnterpriseOnlyErr(err)
			Expect(f).To(BeNil())
		})

	})

	Context("rbac", func() {

		It("should not add filter if rbac config is nil", func() {
			p := NewPlugin()
			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{}, &envoy_config_route.VirtualHost{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("will err if rbac is configured on vhost", func() {
			p := NewPlugin()
			virtualHost := &v1.VirtualHost{
				Name:    "virt1",
				Domains: []string{"*"},
				Options: &v1.VirtualHostOptions{
					Rbac: &rbac.ExtensionSettings{},
				},
			}

			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, virtualHost, &envoy_config_route.VirtualHost{})
			ExpectEnterpriseOnlyErr(err)
		})

		It("will err if rbac is configured on route", func() {
			p := NewPlugin()
			virtualHost := &v1.Route{
				Name: "route1",
				Options: &v1.RouteOptions{
					Rbac: &rbac.ExtensionSettings{},
				},
			}

			err := p.ProcessRoute(plugins.RouteParams{}, virtualHost, &envoy_config_route.Route{})
			ExpectEnterpriseOnlyErr(err)
		})

	})

	Context("sanitize_cluster_header", func() {

		It("should not add filter if sanitize cluster header config is nil", func() {
			p := NewPlugin()
			f, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeNil())
		})

		It("will err if sanitize cluster header is configured", func() {
			p := NewPlugin()
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					SanitizeClusterHeader: &wrappers.BoolValue{},
				},
			}

			f, err := p.HttpFilters(plugins.Params{}, hl)
			ExpectEnterpriseOnlyErr(err)
			Expect(f).To(BeNil())
		})
	})

	Context("waf", func() {

		It("should not add filter if waf config is nil", func() {
			p := NewPlugin()
			f, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeNil())
		})

		It("will err if waf is configured", func() {
			p := NewPlugin()
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					Waf: &waf.Settings{},
				},
			}

			f, err := p.HttpFilters(plugins.Params{}, hl)
			ExpectEnterpriseOnlyErr(err)
			Expect(f).To(BeNil())
		})

		It("will err if waf is configured on vhost", func() {
			p := NewPlugin()
			virtualHost := &v1.VirtualHost{
				Name:    "virt1",
				Domains: []string{"*"},
				Options: &v1.VirtualHostOptions{
					Waf: &waf.Settings{},
				},
			}

			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, virtualHost, &envoy_config_route.VirtualHost{})
			ExpectEnterpriseOnlyErr(err)
		})

		It("will err if waf is configured on route", func() {
			p := NewPlugin()
			virtualHost := &v1.Route{
				Name: "route1",
				Options: &v1.RouteOptions{
					Waf: &waf.Settings{},
				},
			}

			err := p.ProcessRoute(plugins.RouteParams{}, virtualHost, &envoy_config_route.Route{})
			ExpectEnterpriseOnlyErr(err)
		})

	})

	Context("extproc", func() {

		It("should not add filter if extproc config is nil", func() {
			p := NewPlugin()
			f, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeNil())
		})

		It("should error if disable extproc is configured on listener", func() {
			p := NewPlugin()
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					ExtProcConfig: &v1.HttpListenerOptions_DisableExtProc{DisableExtProc: &wrappers.BoolValue{Value: false}},
				},
			}

			f, err := p.HttpFilters(plugins.Params{}, hl)
			ExpectEnterpriseOnlyErr(err)
			Expect(f).To(BeNil())
		})

		It("should error if extproc is configured on listener", func() {
			p := NewPlugin()
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					ExtProcConfig: &v1.HttpListenerOptions_ExtProc{ExtProc: &extproc.Settings{}},
				},
			}

			f, err := p.HttpFilters(plugins.Params{}, hl)
			ExpectEnterpriseOnlyErr(err)
			Expect(f).To(BeNil())
		})

		It("should error if extproc is configured on vhost", func() {
			p := NewPlugin()
			virtualHost := &v1.VirtualHost{
				Name:    "virt1",
				Domains: []string{"*"},
				Options: &v1.VirtualHostOptions{
					ExtProc: &extproc.RouteSettings{},
				},
			}

			err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, virtualHost, &envoy_config_route.VirtualHost{})
			ExpectEnterpriseOnlyErr(err)
		})

		It("should error if extproc is configured on route", func() {
			p := NewPlugin()
			virtualHost := &v1.Route{
				Name: "route1",
				Options: &v1.RouteOptions{
					ExtProc: &extproc.RouteSettings{},
				},
			}

			err := p.ProcessRoute(plugins.RouteParams{}, virtualHost, &envoy_config_route.Route{})
			ExpectEnterpriseOnlyErr(err)
		})

	})

	Context("wasm", func() {

		var (
			p plugins.HttpFilterPlugin
		)

		BeforeEach(func() {
			p = NewPlugin()
		})

		It("should not add filter if wasm config is nil", func() {
			f, err := p.HttpFilters(plugins.Params{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).To(BeNil())
		})

		It("will err if wasm is configured", func() {
			image := "hello"
			hl := &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					Wasm: &wasm.PluginSource{
						Filters: []*wasm.WasmFilter{
							{
								Src: &wasm.WasmFilter_Image{
									Image: image,
								},
							},
						},
					},
				},
			}

			f, err := p.HttpFilters(plugins.Params{}, hl)
			ExpectEnterpriseOnlyErr(err)
			Expect(f).To(BeNil())
		})
	})

	Context("aws", func() {

		It("will not err if aws.WrapAsApiGateway is not configured on single destination route", func() {
			p := NewPlugin()

			route := &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: &core.ResourceRef{
										Namespace: "ns",
										Name:      "upstreamName",
									},
								},
								DestinationSpec: &v1.DestinationSpec{
									DestinationType: &v1.DestinationSpec_Aws{
										Aws: &awsapi.DestinationSpec{
											LogicalName:      "funcName",
											WrapAsApiGateway: false,
										},
									},
								},
							},
						},
					},
				},
			}

			err := p.ProcessRoute(plugins.RouteParams{}, route, &envoy_config_route.Route{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("will err if aws.WrapAsApiGateway is configured on single destination route", func() {
			p := NewPlugin()

			route := &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: &core.ResourceRef{
										Namespace: "ns",
										Name:      "upstreamName",
									},
								},
								DestinationSpec: &v1.DestinationSpec{
									DestinationType: &v1.DestinationSpec_Aws{
										Aws: &awsapi.DestinationSpec{
											LogicalName:      "funcName",
											WrapAsApiGateway: true,
										},
									},
								},
							},
						},
					},
				},
			}

			err := p.ProcessRoute(plugins.RouteParams{}, route, &envoy_config_route.Route{})
			ExpectEnterpriseOnlyErr(err)
		})

		It("will err if aws.WrapAsApiGateway is configured on multi destination route", func() {
			p := NewPlugin()

			route := &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Multi{
							Multi: &v1.MultiDestination{
								Destinations: []*v1.WeightedDestination{{
									Weight: &wrappers.UInt32Value{
										Value: 100,
									},
									Destination: &v1.Destination{
										DestinationType: &v1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Namespace: "ns",
												Name:      "upstreamName",
											},
										},
										DestinationSpec: &v1.DestinationSpec{
											DestinationType: &v1.DestinationSpec_Aws{
												Aws: &awsapi.DestinationSpec{
													LogicalName:      "funcName",
													WrapAsApiGateway: true,
												},
											},
										},
									},
								}},
							},
						},
					},
				},
			}

			err := p.ProcessRoute(plugins.RouteParams{}, route, &envoy_config_route.Route{})
			ExpectEnterpriseOnlyErr(err)
		})

	})

})
