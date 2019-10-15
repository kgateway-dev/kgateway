package sanitizer_test

import (
	"context"
	"net/http"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/errors"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
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

		badUs = &v1.Upstream{
			Metadata: core.Metadata{
				Name:      "bad",
				Namespace: "upstream",
			},
		}
		badCluster = translator.UpstreamToClusterName(badUs.Metadata.Ref())

		missingCluster = "missing_cluster"

		validRouteSingle = route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: clusterName,
					},
				},
			},
		}

		validRouteMulti = route.Route{
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

		missingRouteSingle = route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: missingCluster,
					},
				},
			},
		}

		missingRouteMulti = route.Route{
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

		badRouteSingle = route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: badCluster,
					},
				},
			},
		}

		badRouteMulti = route.Route{
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters: []*route.WeightedCluster_ClusterWeight{
								{
									Name: clusterName,
								},
								{
									Name: badCluster,
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

		fixedRoute = route.Route{
			Action: &route.Route_DirectResponse{
				DirectResponse: &route.DirectResponseAction{
					Status: invalidCfgPolicy.GetInvalidRouteResponseCode(),
					Body: &envoycore.DataSource{
						Specifier: &envoycore.DataSource_InlineString{
							InlineString: invalidCfgPolicy.GetInvalidRouteResponseBody(),
						},
					},
				},
			},
		}

		routeCfgName = "some dirty routes"

		config = &listener.Filter_TypedConfig{}

		// make Consistent() happy
		listener = &envoyapi.Listener{
			FilterChains: []listener.FilterChain{{
				Filters: []listener.Filter{{
					Name:       util.HTTPConnectionManager,
					ConfigType: config,
				}},
			}},
		}
	)
	BeforeEach(func() {
		var err error
		config.TypedConfig, err = types.MarshalAny(&hcm.HttpConnectionManager{
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
			VirtualHosts: []route.VirtualHost{
				{
					Routes: []route.Route{
						validRouteSingle,
						missingRouteSingle,
					},
				},
				{
					Routes: []route.Route{
						missingRouteMulti,
						validRouteMulti,
					},
				},
				{
					Routes: []route.Route{
						badRouteSingle,
						badRouteMulti,
					},
				},
			},
		}

		expectedCfg := &envoyapi.RouteConfiguration{
			Name: routeCfgName,
			VirtualHosts: []route.VirtualHost{
				{
					Routes: []route.Route{
						validRouteSingle,
						fixedRoute,
					},
				},
				{
					Routes: []route.Route{
						fixedRoute,
						validRouteMulti,
					},
				},
				{
					Routes: []route.Route{
						fixedRoute,
						fixedRoute,
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

		sanitizer := NewRouteReplacingSanitizer(invalidCfgPolicy)

		// should have a warning
		reports := reporter.ResourceReports{
			&v1.Proxy{}: {
				Warnings: []string{"route with missing upstream"},
			},
			us: {},
			badUs: {
				Errors: errors.Errorf("don't get me started"),
			},
		}

		glooSnapshot := &v1.ApiSnapshot{
			Upstreams: v1.UpstreamList{us, badUs},
		}

		snap, err := sanitizer.SanitizeSnapshot(context.TODO(), glooSnapshot, xdsSnapshot, reports)
		Expect(err).NotTo(HaveOccurred())

		routeCfgs := snap.GetResources(xds.RouteType)

		sanitizedCfg := routeCfgs.Items[routeCfg.GetName()]

		Expect(sanitizedCfg.ResourceProto()).To(Equal(expectedCfg))
	})
})
