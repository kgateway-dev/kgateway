package sanitizer

import (
	"context"
	"net/http"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("RouteReplacingSanitizer", func() {
	var (
		us = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "my",
				Namespace: "upstream",
			},
		}
		clusterName = translator.UpstreamToClusterName(us.Metadata.Ref())

		missingCluster = "missing_cluster"

		validRouteSingle = &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
						Cluster: clusterName,
					},
				},
			},
		}

		validRouteMulti = &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_config_route_v3.WeightedCluster{
							Clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{
								{
									Name: clusterName,
								},
								{
									Name: clusterName,
								},
							},
						},
					},
				},
			},
		}

		missingRouteSingle = &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
						Cluster: missingCluster,
					},
				},
			},
		}

		fixedRouteSingle = &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
						Cluster: fallbackClusterName,
					},
				},
			},
		}

		missingRouteMulti = &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_config_route_v3.WeightedCluster{
							Clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{
								{
									Name: clusterName,
								},
								{
									Name: missingCluster,
								},
							},
						},
					},
				},
			},
		}

		fixedRouteMulti = &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_config_route_v3.WeightedCluster{
							Clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{
								{
									Name: clusterName,
								},
								{
									Name: fallbackClusterName,
								},
							},
						},
					},
				},
			},
		}

		invalidCfgPolicy = &v1.GlooOptions_InvalidConfigPolicy{
			ReplaceInvalidRoutes:     true,
			InvalidRouteResponseCode: http.StatusTeapot,
			InvalidRouteResponseBody: "out of coffee T_T",
		}

		routeCfgName = "some dirty routes"

		config = &envoy_config_listener_v3.Filter_TypedConfig{}

		// make Consistent() happy
		listener = &envoy_config_listener_v3.Listener{
			FilterChains: []*envoy_config_listener_v3.FilterChain{{
				Filters: []*envoy_config_listener_v3.Filter{{
					Name:       wellknown.HTTPConnectionManager,
					ConfigType: config,
				}},
			}},
		}

		cluster = &envoy_config_cluster_v3.Cluster{
			Name: "my_upstream",
		}

		erroredRouteName = "route-identifier-1"

		erroredRoute = &envoy_config_route_v3.Route{
			Name: erroredRouteName,
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
						Cluster: clusterName,
					},
				},
			},
		}

		fixedErroredRoute = &envoy_config_route_v3.Route{
			Name: erroredRouteName,
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
						Cluster: fallbackClusterName,
					},
				},
			},
		}
	)
	BeforeEach(func() {
		var err error
		config.TypedConfig, err = utils.MessageToAny(&hcm.HttpConnectionManager{
			RouteSpecifier: &hcm.HttpConnectionManager_Rds{
				Rds: &hcm.Rds{
					RouteConfigName: routeCfgName,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})
	It("replaces routes which point to a missing cluster", func() {
		routeCfg := &envoy_config_route_v3.RouteConfiguration{
			Name: routeCfgName,
			VirtualHosts: []*envoy_config_route_v3.VirtualHost{
				{
					Routes: []*envoy_config_route_v3.Route{
						validRouteSingle,
						missingRouteSingle,
					},
				},
				{
					Routes: []*envoy_config_route_v3.Route{
						missingRouteMulti,
						validRouteMulti,
					},
				},
			},
		}

		expectedRoutes := &envoy_config_route_v3.RouteConfiguration{
			Name: routeCfgName,
			VirtualHosts: []*envoy_config_route_v3.VirtualHost{
				{
					Routes: []*envoy_config_route_v3.Route{
						validRouteSingle,
						fixedRouteSingle,
					},
				},
				{
					Routes: []*envoy_config_route_v3.Route{
						fixedRouteMulti,
						validRouteMulti,
					},
				},
			},
		}

		xdsSnapshot := xds.NewSnapshotFromResources(
			envoycache.NewResources("", nil),
			envoycache.NewResources("clusters", []envoycache.Resource{
				resource.NewEnvoyResource(cluster),
			}),
			envoycache.NewResources("routes", []envoycache.Resource{
				resource.NewEnvoyResource(routeCfg),
			}),
			envoycache.NewResources("listeners", []envoycache.Resource{
				resource.NewEnvoyResource(listener),
			}),
		)

		sanitizer, err := NewRouteReplacingSanitizer(invalidCfgPolicy)
		Expect(err).NotTo(HaveOccurred())

		// should have a warning to trigger this sanitizer
		reports := reporter.ResourceReports{
			&v1.Proxy{}: {
				Warnings: []string{"route with missing upstream"},
			},
		}

		glooSnapshot := &v1snap.ApiSnapshot{
			Upstreams: v1.UpstreamList{us},
		}

		snap := sanitizer.SanitizeSnapshot(context.TODO(), glooSnapshot, xdsSnapshot, reports)

		routeCfgs := snap.GetResources(types.RouteTypeV3)
		listeners := snap.GetResources(types.ListenerTypeV3)
		clusters := snap.GetResources(types.ClusterTypeV3)

		sanitizedRoutes := routeCfgs.Items[routeCfg.GetName()]
		listenersWithFallback := listeners.Items[fallbackListenerName]
		clustersWithFallback := clusters.Items[fallbackClusterName]

		Expect(sanitizedRoutes.ResourceProto()).To(matchers.MatchProto(expectedRoutes))
		Expect(listenersWithFallback.ResourceProto()).To(Equal(sanitizer.fallbackListener))
		Expect(clustersWithFallback.ResourceProto()).To(Equal(sanitizer.fallbackCluster))
	})
	It("replaces routes that have errored", func() {
		var multiErr *multierror.Error
		baseError := eris.Errorf("abc. Reason: plugin. %s: %s", validationutils.RouteIdentifierTxt, erroredRouteName)
		multiErr = multierror.Append(
			multiErr,
			eris.Wrap(baseError, validationutils.RouteErrorMsg),
		)
		routeCfg := &envoy_config_route_v3.RouteConfiguration{
			Name: routeCfgName,
			VirtualHosts: []*envoy_config_route_v3.VirtualHost{
				{
					Routes: []*envoy_config_route_v3.Route{
						erroredRoute,
					},
				},
			},
		}
		expectedRoutes := &envoy_config_route_v3.RouteConfiguration{
			Name: routeCfgName,
			VirtualHosts: []*envoy_config_route_v3.VirtualHost{
				{
					Routes: []*envoy_config_route_v3.Route{
						fixedErroredRoute,
					},
				},
			},
		}

		proxy := &v1.Proxy{}
		reports := reporter.ResourceReports{
			proxy: {
				Errors: multiErr,
			},
		}

		xdsSnapshot := xds.NewSnapshotFromResources(
			envoycache.NewResources("", nil),
			envoycache.NewResources("", nil),
			envoycache.NewResources("routes", []envoycache.Resource{
				resource.NewEnvoyResource(routeCfg),
			}),
			envoycache.NewResources("listeners", []envoycache.Resource{
				resource.NewEnvoyResource(listener),
			}),
		)

		sanitizer, err := NewRouteReplacingSanitizer(invalidCfgPolicy)
		Expect(err).NotTo(HaveOccurred())

		glooSnapshot := &v1snap.ApiSnapshot{
			Upstreams: v1.UpstreamList{us},
		}

		snap := sanitizer.SanitizeSnapshot(context.TODO(), glooSnapshot, xdsSnapshot, reports)

		routeCfgs := snap.GetResources(types.RouteTypeV3)
		listeners := snap.GetResources(types.ListenerTypeV3)
		clusters := snap.GetResources(types.ClusterTypeV3)

		sanitizedRoutes := routeCfgs.Items[routeCfg.GetName()]
		listenersWithFallback := listeners.Items[fallbackListenerName]
		clustersWithFallback := clusters.Items[fallbackClusterName]

		Expect(sanitizedRoutes.ResourceProto()).To(matchers.MatchProto(expectedRoutes))
		Expect(listenersWithFallback.ResourceProto()).To(Equal(sanitizer.fallbackListener))
		Expect(clustersWithFallback.ResourceProto()).To(Equal(sanitizer.fallbackCluster))

		// Verify that errors have been turned into warnings
		Expect(reports[proxy].Errors).To(BeNil())
		Expect(reports[proxy].Warnings).To(Equal([]string{multiErr.Errors[0].Error()}))
	})
})
