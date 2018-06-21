package translator_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/internal/control-plane/snapshot"
	v12 "github.com/solo-io/gloo/pkg/api/defaults/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/route-extensions"
	"github.com/solo-io/gloo/pkg/coreplugins/static"

	. "github.com/solo-io/gloo/internal/control-plane/translator"
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

var _ = Describe("Translator", func() {
	It("works", func() {
		t := NewTranslator(nil)
		role := v12.GatewayRole("::", 8080, 8443)
		cfg := ValidConfigSsl()
		v12.AssignGatewayVirtualServices(role.Listeners[0], role.Listeners[1], cfg.VirtualServices)
		snap, reports := t.Translate(role, &snapshot.Cache{Cfg: cfg})
		Expect(snap).To(ContainSubstring(""))
		Expect(reports).To(ContainSubstring(""))
	})
})

func ValidConfigSsl() *v1.Config {
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
	virtualServices := []*v1.VirtualService{
		{
			Name: "valid-vservice",
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
					Extensions: extensions.EncodeRouteExtensionSpec(extensions.RouteExtensionSpec{
						MaxRetries: 2,
						Timeout:    time.Minute,
						AddRequestHeaders: []extensions.HeaderValue{
							{Key: "x-foo", Value: "bar"},
						},
						AddResponseHeaders: []extensions.HeaderValue{
							{Key: "x-foo", Value: "bar"},
						},
						RemoveResponseHeaders: []string{
							"x-bar",
						},
					}),
				},
			},
			SslConfig: &v1.SSLConfig{
				SecretRef: "ssl-secret-ref",
			},
		},
	}
	return &v1.Config{
		Upstreams:       upstreams,
		VirtualServices: virtualServices,
	}
}

func PartiallyValidConfig() *v1.Config {
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
		{
			Name: "invalid-service",
			Type: static.UpstreamTypeService,
			Spec: static.EncodeUpstreamSpec(static.UpstreamSpec{
				Hosts: []static.Host{
					{
						Addr: "", //not a valid addr
						Port: 1234,
					},
				},
			}),
		},
	}
	virtualServices := []*v1.VirtualService{
		{
			Name:    "valid-vService",
			Domains: []string{"foo.example.com"},
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
				},
			},
		},
		{
			Name:    "invalid-vService",
			Domains: []string{"bar.example.com"},
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
						DestinationType: &v1.Destination_Function{
							Function: &v1.FunctionDestination{
								FunctionName: "invalid-func",
								UpstreamName: "invalid-service",
							},
						},
					},
					PrefixRewrite: "/bar",
					Extensions: extensions.EncodeRouteExtensionSpec(extensions.RouteExtensionSpec{
						MaxRetries: 2,
						Timeout:    time.Minute,
						AddRequestHeaders: []extensions.HeaderValue{
							{Key: "x-foo", Value: "bar"},
						},
						AddResponseHeaders: []extensions.HeaderValue{
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
