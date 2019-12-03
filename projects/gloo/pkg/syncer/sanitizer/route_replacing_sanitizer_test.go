package sanitizer

import (
	"context"
	"net/http"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/golang/protobuf/ptypes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = Describe("RouteReplacingSanitizer", func() {
	var (
		us = &v1.Upstream{
			Metadata: core.Metadata{
				Name:      "my",
				Namespace: "upstream",
			},
		}
		clusterName = translator.UpstreamToClusterName(us.Metadata.Ref())

		missingCluster = "missing_cluster"

		validRouteSingle = &route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: clusterName,
					},
				},
			},
		}

		validRouteMulti = &route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{
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

		missingRouteSingle = &route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: missingCluster,
					},
				},
			},
		}

		fixedRouteSingle = &route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: fallbackClusterName,
					},
				},
			},
		}

		missingRouteMulti = &route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{
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

		fixedRouteMulti = &route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{
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

		config = &listener.Filter_TypedConfig{}

		// make Consistent() happy
		listener = &envoyapi.Listener{
			FilterChains: []*listener.FilterChain{{
				Filters: []*listener.Filter{{
					Name:       util.HTTPConnectionManager,
					ConfigType: config,
				}},
			}},
		}
	)
	BeforeEach(func() {
		var err error
		config.TypedConfig, err = ptypes.MarshalAny(&hcm.HttpConnectionManager{
			RouteSpecifier: &hcm.HttpConnectionManager_Rds{
				Rds: &hcm.Rds{
					RouteConfigName: routeCfgName,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})
	It("replaces routes which point to a missing cluster", func() {
		routeCfg := &envoyapi.RouteConfiguration{
			Name: routeCfgName,
			VirtualHosts: []*route.VirtualHost{
				{
					Routes: []*route.Route{
						validRouteSingle,
						missingRouteSingle,
					},
				},
				{
					Routes: []*route.Route{
						missingRouteMulti,
						validRouteMulti,
					},
				},
			},
		}

		expectedRoutes := &envoyapi.RouteConfiguration{
			Name: routeCfgName,
			VirtualHosts: []*route.VirtualHost{
				{
					Routes: []*route.Route{
						validRouteSingle,
						fixedRouteSingle,
					},
				},
				{
					Routes: []*route.Route{
						fixedRouteMulti,
						validRouteMulti,
					},
				},
			},
		}

		xdsSnapshot := xds.NewSnapshotFromResources(
			envoycache.NewResources("", nil),
			envoycache.NewResources("", nil),
			envoycache.NewResources("routes", []envoycache.Resource{
				xds.NewEnvoyResource(routeCfg),
			}),
			envoycache.NewResources("listeners", []envoycache.Resource{
				xds.NewEnvoyResource(listener),
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

		glooSnapshot := &v1.ApiSnapshot{
			Upstreams: v1.UpstreamList{us},
		}

		snap, err := sanitizer.SanitizeSnapshot(context.TODO(), glooSnapshot, xdsSnapshot, reports)
		Expect(err).NotTo(HaveOccurred())

		routeCfgs := snap.GetResources(xds.RouteType)
		listeners := snap.GetResources(xds.ListenerType)
		clusters := snap.GetResources(xds.ClusterType)

		sanitizedRoutes := routeCfgs.Items[routeCfg.GetName()]
		listenersWithFallback := listeners.Items[fallbackListenerName]
		clustersWithFallback := clusters.Items[fallbackClusterName]

		Expect(sanitizedRoutes.ResourceProto()).To(Equal(expectedRoutes))
		Expect(listenersWithFallback.ResourceProto()).To(Equal(sanitizer.fallbackListener))
		Expect(clustersWithFallback.ResourceProto()).To(Equal(sanitizer.fallbackCluster))
	})
})
