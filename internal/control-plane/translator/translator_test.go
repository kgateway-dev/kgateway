package translator

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"

	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/internal/control-plane/bootstrap"
	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/internal/control-plane/snapshot"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/route-extensions"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

func newTranslator() *Translator {
	return NewTranslator(bootstrap.IngressOptions{"::", 8080, 8443}, []plugins.TranslatorPlugin{service.NewPlugin()})
}

var _ = Describe("Translator", func() {
	role := &v1.Role{Name: "myrole"}
	Context("invalid config", func() {
		Context("domains are not unique amongst virtual services", func() {
			cfg := InvalidConfigSharedDomains()
			t := newTranslator()
			snap, reports, err := t.Translate(role, &snapshot.Cache{Cfg: cfg})
			It("returns five reports, one for each upstream, one for each virtualservice", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(reports).To(HaveLen(4))
				Expect(reports[0].CfgObject).To(Equal(cfg.Upstreams[0]))
				Expect(reports[1].CfgObject).To(Equal(cfg.VirtualServices[0]))
				Expect(reports[2].CfgObject).To(Equal(cfg.VirtualServices[1]))
				Expect(reports[3].CfgObject).To(Equal(role))
			})
			It("returns no error report for the cluster", func() {
				Expect(reports[0].Err).To(BeNil())
			})
			It("returns a valid report for each virtual service, and an error for the role", func() {
				Expect(reports[1].Err).NotTo(BeNil())
				Expect(reports[2].Err).NotTo(BeNil())
				Expect(reports[3]).NotTo(BeNil())
				Expect(reports[1].Err.Error()).To(ContainSubstring("is shared by the following virtual services: [invalid-vservice-1 invalid-vservice-2]"))
				Expect(reports[2].Err.Error()).To(ContainSubstring("is shared by the following virtual services: [invalid-vservice-1 invalid-vservice-2]"))
				Expect(reports[3].Err.Error()).To(ContainSubstring("is shared by the following virtual services: [invalid-vservice-1 invalid-vservice-2]"))
			})
			It("returns only the valid cluster", func() {
				clas, clusters, routeConfigs, listeners := getSnapshotResources(snap)
				Expect(clas).To(HaveLen(0))
				Expect(clusters).To(HaveLen(1))
				Expect(routeConfigs).To(HaveLen(0))
				Expect(listeners).To(HaveLen(0))
			})
		})
		Context("one valid route, one invalid route", func() {
			cfg := PartiallyValidConfig()
			t := newTranslator()
			snap, reports, err := t.Translate(role, &snapshot.Cache{Cfg: cfg})
			It("returns four reports, one for each upstream, one for each virtualservice", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(reports).To(HaveLen(5))
				Expect(reports[0].CfgObject).To(Equal(cfg.Upstreams[0]))
				Expect(reports[1].CfgObject).To(Equal(cfg.Upstreams[1]))
				Expect(reports[2].CfgObject).To(Equal(cfg.VirtualServices[0]))
				Expect(reports[3].CfgObject).To(Equal(cfg.VirtualServices[1]))
				Expect(reports[4].CfgObject).To(Equal(role))
			})
			It("returns an error report for the bad cluster and the bad route", func() {
				Expect(reports[0].Err).To(BeNil())
				Expect(reports[1].Err).NotTo(BeNil())
				Expect(reports[2].Err).To(BeNil())
				Expect(reports[3].Err).NotTo(BeNil())
				Expect(reports[4].Err).To(BeNil())
				Expect(reports[1].Err.Error()).To(ContainSubstring("ip cannot be empty"))
				Expect(reports[3].Err.Error()).To(ContainSubstring("upstream invalid-service was not found or had errors for function destination"))
			})
			It("returns one cluster and one envoy virtual host", func() {
				clas, clusters, routeConfigs, listeners := getSnapshotResources(snap)
				Expect(clas).To(HaveLen(0))
				Expect(clusters).To(HaveLen(1))
				Expect(routeConfigs).To(HaveLen(1))
				Expect(routeConfigs[0].VirtualHosts).To(HaveLen(1))
				Expect(listeners).To(HaveLen(1))
				Expect(listeners[0].FilterChains).To(HaveLen(1))
				Expect(listeners[0].FilterChains[0].Filters).To(HaveLen(1))
			})
		})
		Context("with missing upstream for route", func() {
			cfg := InvalidConfigNoUpstream()
			t := newTranslator()
			It("returns report for the error and no virtual services", func() {
				snap, reports, err := t.Translate(role, &snapshot.Cache{Cfg: cfg})
				Expect(err).NotTo(HaveOccurred())
				Expect(reports).To(HaveLen(3))
				Expect(reports[0].CfgObject).To(Equal(cfg.Upstreams[0]))
				Expect(reports[0].Err).To(BeNil())
				Expect(reports[1].Err).NotTo(BeNil())
				Expect(reports[1].Err.Error()).To(ContainSubstring("upstream invalid-service was not found or had " +
					"errors for upstream destination"))
				Expect(reports[1].CfgObject).To(Equal(cfg.VirtualServices[0]))
				Expect(reports[2].Err).To(BeNil())
				Expect(reports[2].CfgObject).To(Equal(role))
				clas, clusters, routeConfigs, listeners := getSnapshotResources(snap)
				Expect(clas).To(HaveLen(0))
				Expect(clusters).To(HaveLen(1))
				Expect(routeConfigs).To(HaveLen(0))
				Expect(listeners).To(HaveLen(0))
			})
		})
	})
	Context("valid config", func() {
		Context("with no ssl vServices", func() {
			cfg := ValidConfigNoSsl()
			t := newTranslator()
			It("returns an empty ssl routeconfig and a len 1 nossl routeconfig", func() {
				snap, reports, err := t.Translate(role, &snapshot.Cache{Cfg: cfg})
				Expect(err).NotTo(HaveOccurred())
				Expect(reports).To(HaveLen(3))
				Expect(reports[0].CfgObject).To(Equal(cfg.Upstreams[0]))
				Expect(reports[1].CfgObject).To(Equal(cfg.VirtualServices[0]))
				Expect(reports[2].CfgObject).To(Equal(role))
				Expect(reports[0].Err).To(BeNil())
				Expect(reports[1].Err).To(BeNil())
				Expect(reports[2].Err).To(BeNil())
				clas, clusters, routeConfigs, listeners := getSnapshotResources(snap)
				Expect(clas).To(HaveLen(0))
				Expect(clusters).To(HaveLen(1))
				Expect(routeConfigs).To(HaveLen(1))
				Expect(routeConfigs[0].Name).To(Equal(noSslRdsName))
				Expect(routeConfigs[0].VirtualHosts).To(HaveLen(1))
				Expect(routeConfigs[0].VirtualHosts[0].RequireTls).To(Equal(envoyroute.VirtualHost_NONE))
				Expect(listeners).To(HaveLen(1))
			})
		})
		Context("with an ssl secret specified", func() {
			cfg := ValidConfigSsl()
			t := newTranslator()
			Context("the desired ssl secret not present in the secret map", func() {
				It("returns an error for the not found secretref", func() {
					_, reports, err := t.Translate(role, &snapshot.Cache{Cfg: cfg})
					Expect(err).NotTo(HaveOccurred())
					Expect(reports).To(HaveLen(3))
					Expect(reports[0].CfgObject).To(Equal(cfg.Upstreams[0]))
					Expect(reports[1].CfgObject).To(Equal(cfg.VirtualServices[0]))
					Expect(reports[2].CfgObject).To(Equal(role))
					Expect(reports[0].Err).To(BeNil())
					Expect(reports[1].Err).NotTo(BeNil())
					Expect(reports[1].Err.Error()).To(ContainSubstring("secret not found for ref ssl-secret-ref"))
					Expect(reports[2].Err).To(BeNil())
				})
			})
			Context("the desired ssl secret is present in the secret map", func() {
				var (
					err     error
					reports []reporter.ConfigObjectReport
					snap    *envoycache.Snapshot
				)
				It("returns a non empty ssl routeconfig and a len 1 nossl routeconfig", func() {
					snap, reports, err = t.Translate(role, &snapshot.Cache{
						Cfg: cfg,
						Secrets: secretwatcher.SecretMap{
							"ssl-secret-ref": &dependencies.Secret{Ref: "ssl-secret-ref", Data: map[string]string{
								"ca_chain":    "1111",
								"private_key": "1111",
							},
							}},
					})
				})

				It("returns a non empty ssl routeconfig and a len 1 nossl routeconfig", func() {
					snap, reports, err = t.Translate(role, &snapshot.Cache{
						Cfg: cfg,
						Secrets: secretwatcher.SecretMap{
							"ssl-secret-ref": &dependencies.Secret{Ref: "ssl-secret-ref", Data: map[string]string{
								"tls.crt": "1111",
								"tls.key": "1111",
							},
							}},
					})
				})

				AfterEach(func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(reports).To(HaveLen(3))
					Expect(reports[0].CfgObject).To(Equal(cfg.Upstreams[0]))
					Expect(reports[1].CfgObject).To(Equal(cfg.VirtualServices[0]))
					Expect(reports[2].CfgObject).To(Equal(role))
					Expect(reports[0].Err).To(BeNil())
					Expect(reports[1].Err).To(BeNil())
					Expect(reports[2].Err).To(BeNil())
					clas, clusters, routeConfigs, listeners := getSnapshotResources(snap)
					Expect(clas).To(HaveLen(0))
					Expect(clusters).To(HaveLen(1))
					Expect(routeConfigs).To(HaveLen(1))
					Expect(routeConfigs[0].Name).To(Equal(sslRdsName))
					Expect(routeConfigs[0].VirtualHosts).To(HaveLen(1))
					Expect(routeConfigs[0].VirtualHosts[0].RequireTls).To(Equal(envoyroute.VirtualHost_ALL))
					Expect(listeners).To(HaveLen(1))
				})
			})
		})
	})
})

func getSnapshotResources(snap *envoycache.Snapshot) ([]*v2.ClusterLoadAssignment, []*v2.Cluster, []*v2.RouteConfiguration, []*v2.Listener) {

	var (
		clas         []*v2.ClusterLoadAssignment
		clusters     []*v2.Cluster
		routeConfigs []*v2.RouteConfiguration
		listeners    []*v2.Listener
	)
	for _, pb := range snap.Endpoints.Items {
		clas = append(clas, pb.(*v2.ClusterLoadAssignment))
	}
	for _, pb := range snap.Clusters.Items {
		clusters = append(clusters, pb.(*v2.Cluster))
	}
	for _, pb := range snap.Routes.Items {
		routeConfigs = append(routeConfigs, pb.(*v2.RouteConfiguration))
	}
	for _, pb := range snap.Listeners.Items {
		listeners = append(listeners, pb.(*v2.Listener))
	}
	return clas, clusters, routeConfigs, listeners
}

func ValidConfigNoSsl() *v1.Config {
	upstreams := []*v1.Upstream{
		{
			Name: "valid-service",
			Type: service.UpstreamTypeService,
			Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
				Hosts: []service.Host{
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
		},
	}
	return &v1.Config{
		Upstreams:       upstreams,
		VirtualServices: virtualServices,
	}
}

func InvalidConfigSharedDomains() *v1.Config {
	upstreams := []*v1.Upstream{
		{
			Name: "valid-service",
			Type: service.UpstreamTypeService,
			Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
				Hosts: []service.Host{
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
			Name: "invalid-vservice-1",
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
			Name:    "invalid-vservice-2",
			Domains: []string{"*"},
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
	}
	return &v1.Config{
		Upstreams:       upstreams,
		VirtualServices: virtualServices,
	}
}

func ValidConfigSsl() *v1.Config {
	upstreams := []*v1.Upstream{
		{
			Name: "valid-service",
			Type: service.UpstreamTypeService,
			Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
				Hosts: []service.Host{
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
			Type: service.UpstreamTypeService,
			Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
				Hosts: []service.Host{
					{
						Addr: "localhost",
						Port: 1234,
					},
				},
			}),
		},
		{
			Name: "invalid-service",
			Type: service.UpstreamTypeService,
			Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
				Hosts: []service.Host{
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

func InvalidConfigNoFuncPlugin() *v1.Config {
	upstreams := []*v1.Upstream{
		{
			Name: "valid-service",
			Type: service.UpstreamTypeService,
			Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
				Hosts: []service.Host{
					{
						Addr: "localhost",
						Port: 1234,
					},
				},
			}),
			Functions: []*v1.Function{
				{
					Name: "invalid-func",
				},
			},
		},
	}
	virtualServices := []*v1.VirtualService{
		{
			Name: "invalid-vservice",
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
								UpstreamName: "valid-service",
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

func InvalidConfigNoUpstream() *v1.Config {
	upstreams := []*v1.Upstream{
		{
			Name: "valid-service",
			Type: service.UpstreamTypeService,
			Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
				Hosts: []service.Host{
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
			Name: "invalid-vservice",
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
								Name: "invalid-service",
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
