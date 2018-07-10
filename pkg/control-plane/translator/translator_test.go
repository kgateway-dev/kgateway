package translator_test

import (
	"sort"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/control-plane/snapshot"
	v12 "github.com/solo-io/gloo/pkg/api/defaults/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/routing"
	"github.com/solo-io/gloo/pkg/coreplugins/static"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/pkg/storage/dependencies"

	. "github.com/solo-io/gloo/pkg/control-plane/translator"
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

var _ = Describe("Translator", func() {
	It("works", func() {
		t := NewTranslator(nil)
		role := v12.GatewayRole("::", 8080, 8443)
		cfg := ValidConfig()
		v12.AssignGatewayVirtualServices(role.Listeners[0], role.Listeners[1], cfg.VirtualServices)
		_, reports := t.Translate(role, &snapshot.Cache{Cfg: cfg})
		Expect(reports).To(HaveLen(4))
		sort.SliceStable(reports, func(i, j int) bool {
			return reports[i].CfgObject.GetName() < reports[j].CfgObject.GetName()
		})
		Expect(reports[0].CfgObject).To(Equal(role))
		Expect(reports[0].Err).To(BeNil())

		Expect(reports[1].CfgObject).To(Equal(cfg.Upstreams[0]))
		Expect(reports[1].Err).To(BeNil())

		Expect(reports[2].CfgObject).To(Equal(cfg.VirtualServices[0]))
		Expect(reports[2].Err).NotTo(BeNil())
		Expect(reports[2].Err.Error()).To(ContainSubstring("ssl secret not found for ref ssl-secret-ref"))

		Expect(reports[3].CfgObject).To(Equal(cfg.VirtualServices[1]))
		Expect(reports[3].Err).To(BeNil())

		// fix secrets
		snap, reports := t.Translate(role, &snapshot.Cache{Cfg: cfg, Secrets: secretwatcher.SecretMap{
			"ssl-secret-ref": &dependencies.Secret{
				Ref: "ssl-secret-ref",
				Data: map[string]string{
					"tls.crt": "asdf",
					"tls.key": "asdf",
				},
			},
		}})
		Expect(reports).To(HaveLen(4))
		sort.SliceStable(reports, func(i, j int) bool {
			return reports[i].CfgObject.GetName() < reports[j].CfgObject.GetName()
		})
		Expect(reports[0].CfgObject).To(Equal(role))
		Expect(reports[0].Err).To(BeNil())

		Expect(reports[1].CfgObject).To(Equal(cfg.Upstreams[0]))
		Expect(reports[1].Err).To(BeNil())

		Expect(reports[2].CfgObject).To(Equal(cfg.VirtualServices[0]))
		Expect(reports[2].Err).To(BeNil())

		Expect(reports[3].CfgObject).To(Equal(cfg.VirtualServices[1]))
		Expect(reports[3].Err).To(BeNil())

		Expect(snap.Clusters.Items).To(HaveKey("valid-service"))
		Expect(snap.Clusters.Items["valid-service"]).NotTo(BeNil())
		Expect(snap.Clusters.Items["valid-service"].String()).To(ContainSubstring(`name:"valid-service" type:STRICT_DNS connect_timeout:<seconds:5 > hosts:<socket_address:<address:"localhost" port_value:1234 > > dns_lookup_family:V4_ONLY metadata:<> `))

		Expect(snap.Listeners.Items).To(HaveKey("insecure-gateway-listener"))
		Expect(snap.Listeners.Items["insecure-gateway-listener"]).NotTo(BeNil())
		Expect(snap.Listeners.Items["insecure-gateway-listener"].String()).To(ContainSubstring(`"insecure-gateway-listener" address:<socket_address:<address:"::" port_value:8080 ipv4_compat:true > > filter_chains:<filters:<name:"envoy.http_connection_manager" config:<fields:<key:"http_filters" value:<list_value:<values:<struct_value:<fields:<key:"name" value:<string_value:"envoy.router" > > > > > > > fields:<key:"rds" value:<struct_value:<fields:<key:"config_source" value:<struct_value:<fields:<key:"ads" value:<struct_value:<> > > > > > fields:<key:"route_config_name" value:<string_value:"insecure-gateway-listener-routes" > > > > > fields:<key:"stat_prefix" value:<string_value:"http" > > > > > `))

		Expect(snap.Routes.Items).To(HaveKey("insecure-gateway-listener-routes"))
		Expect(snap.Routes.Items["insecure-gateway-listener-routes"]).NotTo(BeNil())
		Expect(snap.Routes.Items["insecure-gateway-listener-routes"].String()).To(ContainSubstring(`name:"insecure-gateway-listener-routes" virtual_hosts:<name:"valid-vservice-2" domains:"*" routes:<match:<prefix:"/foo" headers:<name:"x-foo-bar" value:".*" regex:<value:true > > headers:<name:":method" value:"GET|POST" regex:<value:true > > > route:<cluster:"valid-service" prefix_rewrite:"/bar" auto_host_rewrite:<value:true > timeout:<seconds:60 > retry_policy:<retry_on:"5xx" num_retries:<value:2 > > request_headers_to_add:<header:<key:"x-foo" value:"bar" > append:<> > response_headers_to_add:<header:<key:"x-foo" value:"bar" > append:<> > response_headers_to_remove:"x-bar" > > >`))

		Expect(snap.Endpoints.Items).To(HaveLen(0))
	})
})

func ValidConfig() *v1.Config {
	upstreams := []*v1.Upstream{
		{
			Name: "valid-service",
			Type: static.UpstreamTypeService,
			Spec: static.EncodeUpstreamSpec(static.UpstreamSpec{
				Hosts: []static.Host{
					{
						Addr: "localhost",
						Port: 1234,
					},
				},
			}),
		},
	}
	timeout := time.Minute
	virtualServices := []*v1.VirtualService{
		{
			Name: "valid-vservice-1",
			Routes: []*v1.Route{
				{
					Matcher: &v1.Route_RequestMatcher{
						RequestMatcher: &v1.RequestMatcher{
							Path: &v1.RequestMatcher_PathPrefix{
								PathPrefix: "/foo",
							},
							Headers: map[string]string{"x-foo-bar": ""},
							Verbs:   []string{"GET", "POST"},
						},
					},
					SingleDestination: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &v1.UpstreamDestination{
								Name: "valid-service",
							},
						},
					},
					PrefixRewrite: "/bar",
					Extensions: routing.EncodeRouteExtensionSpec(routing.RouteExtensions{
						MaxRetries: 2,
						Timeout:    &timeout,
						AddRequestHeaders: []*routing.HeaderValue{
							{Key: "x-foo", Value: "bar"},
						},
						AddResponseHeaders: []*routing.HeaderValue{
							{Key: "x-foo", Value: "bar"},
						},
						RemoveResponseHeaders: []string{
							"x-bar",
						},
					}),
				},
			},
			SslConfig: &v1.SSLConfig{
				SslSecrets: &v1.SSLConfig_SecretRef{SecretRef: "ssl-secret-ref"},
			},
		},
		{
			Name: "valid-vservice-2",
			Routes: []*v1.Route{
				{
					Matcher: &v1.Route_RequestMatcher{
						RequestMatcher: &v1.RequestMatcher{
							Path: &v1.RequestMatcher_PathPrefix{
								PathPrefix: "/foo",
							},
							Headers: map[string]string{"x-foo-bar": ""},
							Verbs:   []string{"GET", "POST"},
						},
					},
					SingleDestination: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &v1.UpstreamDestination{
								Name: "valid-service",
							},
						},
					},
					PrefixRewrite: "/bar",
					Extensions: routing.EncodeRouteExtensionSpec(routing.RouteExtensions{
						MaxRetries: 2,
						Timeout:    &timeout,
						AddRequestHeaders: []*routing.HeaderValue{
							{Key: "x-foo", Value: "bar"},
						},
						AddResponseHeaders: []*routing.HeaderValue{
							{Key: "x-foo", Value: "bar"},
						},
						RemoveResponseHeaders: []string{
							"x-bar",
						},
					}),
				},
			},
		},
	}
	return &v1.Config{
		Upstreams:       upstreams,
		VirtualServices: virtualServices,
	}
}
