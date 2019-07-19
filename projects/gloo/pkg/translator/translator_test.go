package translator_test

import (
	"context"
	"fmt"

	"github.com/golang/mock/gomock"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/projects/gloo/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"

	envoyrouteapi "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/proto"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	k8scorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/solo-io/gloo/pkg/utils"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	envoycluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"
	v1grpc "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc"
	v1kubernetes "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Translator", func() {
	var (
		ctrl              *gomock.Controller
		settings          *v1.Settings
		translator        Translator
		upstream          *v1.Upstream
		proxy             *v1.Proxy
		params            plugins.Params
		registeredPlugins []plugins.Plugin
		matcher           *v1.Matcher
		routes            []*v1.Route

		snapshot           envoycache.Snapshot
		cluster            *envoyapi.Cluster
		listener           *envoyapi.Listener
		hcmCfg             *envoyhttp.HttpConnectionManager
		routeConfiguration *envoyapi.RouteConfiguration
	)

	BeforeEach(func() {

		ctrl = gomock.NewController(T)

		cluster = nil
		settings = &v1.Settings{}
		memoryClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
		opts := bootstrap.Opts{
			Settings:  settings,
			Secrets:   memoryClientFactory,
			Upstreams: memoryClientFactory,
			ConsulClient: consul.NewMockConsulWatcher(ctrl), // just needed to activate the consul plugin
		}
		registeredPlugins = registry.Plugins(opts)

		upname := core.Metadata{
			Name:      "test",
			Namespace: "gloo-system",
		}
		upstream = &v1.Upstream{
			Metadata: upname,
			UpstreamSpec: &v1.UpstreamSpec{
				UpstreamType: &v1.UpstreamSpec_Static{
					Static: &v1static.UpstreamSpec{
						Hosts: []*v1static.Host{
							{
								Addr: "Test",
								Port: 124,
							},
						},
					},
				},
			},
		}

		params = plugins.Params{
			Ctx: context.Background(),
			Snapshot: &v1.ApiSnapshot{
				Upstreams: v1.UpstreamList{
					upstream,
				},
			},
		}
		matcher = &v1.Matcher{
			PathSpecifier: &v1.Matcher_Prefix{
				Prefix: "/",
			},
		}
		routes = []*v1.Route{{
			Matcher: matcher,
			Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: utils.ResourceRefPtr(upname.Ref()),
							},
						},
					},
				},
			},
		}}
	})

	JustBeforeEach(func() {
		translator = NewTranslator(registeredPlugins, settings)
		proxy = &v1.Proxy{
			Metadata: core.Metadata{
				Name:      "test",
				Namespace: "gloo-system",
			},
			Listeners: []*v1.Listener{{
				Name:        "listener",
				BindAddress: "127.0.0.1",
				BindPort:    80,
				ListenerType: &v1.Listener_HttpListener{
					HttpListener: &v1.HttpListener{
						VirtualHosts: []*v1.VirtualHost{{
							Name:    "virt1",
							Domains: []string{"*"},
							Routes:  routes,
						}},
					},
				},
			}},
		}
	})

	translate := func() {

		snap, errs, err := translator.Translate(params, proxy)
		Expect(err).NotTo(HaveOccurred())
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())

		clusters := snap.GetResources(xds.ClusterType)
		clusterResource := clusters.Items[UpstreamToClusterName(upstream.Metadata.Ref())]
		cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
		Expect(cluster).NotTo(BeNil())

		listeners := snap.GetResources(xds.ListenerType)
		listenerResource := listeners.Items["listener"]
		listener = listenerResource.ResourceProto().(*envoyapi.Listener)
		Expect(listener).NotTo(BeNil())

		hcmFilter := listener.FilterChains[0].Filters[0]
		hcmCfg = &envoyhttp.HttpConnectionManager{}
		err = ParseConfig(&hcmFilter, hcmCfg)
		Expect(err).NotTo(HaveOccurred())

		routes := snap.GetResources(xds.RouteType)
		Expect(routes.Items).To(HaveKey("listener-routes"))
		routeResource := routes.Items["listener-routes"]
		routeConfiguration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
		Expect(routeConfiguration).NotTo(BeNil())

		snapshot = snap
	}

	It("sanitizes an invalid virtual host name", func() {
		proxyClone := proto.Clone(proxy).(*v1.Proxy)
		proxyClone.GetListeners()[0].GetHttpListener().GetVirtualHosts()[0].Name = "invalid.name"

		snap, errs, err := translator.Translate(params, proxyClone)

		Expect(err).NotTo(HaveOccurred())
		Expect(errs.Validate()).NotTo(HaveOccurred())
		Expect(snap).NotTo(BeNil())

		routes := snap.GetResources(xds.RouteType)
		Expect(routes.Items).To(HaveKey("listener-routes"))
		routeResource := routes.Items["listener-routes"]
		routeConfiguration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
		Expect(routeConfiguration).NotTo(BeNil())
		Expect(routeConfiguration.GetVirtualHosts()).To(HaveLen(1))
		Expect(routeConfiguration.GetVirtualHosts()[0].Name).To(Equal("invalid_name"))
	})

	Context("service spec", func() {
		It("changes in service spec should create a different snapshot", func() {
			translate()
			oldVersion := snapshot.GetResources(xds.ClusterType).Version

			svcSpec := &v1plugins.ServiceSpec{
				PluginType: &v1plugins.ServiceSpec_Grpc{
					Grpc: &v1grpc.ServiceSpec{},
				},
			}
			upstream.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Static).SetServiceSpec(svcSpec)
			translate()
			newVersion := snapshot.GetResources(xds.ClusterType).Version
			Expect(oldVersion).ToNot(Equal(newVersion))
		})
	})

	Context("route header match", func() {
		It("should translate header matcher with no value to a PresentMatch", func() {

			matcher.Headers = []*v1.HeaderMatcher{
				{
					Name: "test",
				},
			}
			translate()
			headermatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headermatch.Name).To(Equal("test"))
			presentmatch := headermatch.GetPresentMatch()
			Expect(presentmatch).To(BeTrue())
		})

		It("should translate header matcher with value to exact match", func() {

			matcher.Headers = []*v1.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
				},
			}
			translate()

			headermatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headermatch.Name).To(Equal("test"))
			exactmatch := headermatch.GetExactMatch()
			Expect(exactmatch).To(Equal("testvalue"))
		})

		It("should translate header matcher with regex becomes regex match", func() {

			matcher.Headers = []*v1.HeaderMatcher{
				{
					Name:  "test",
					Value: "testvalue",
					Regex: true,
				},
			}
			translate()

			headermatch := routeConfiguration.VirtualHosts[0].Routes[0].Match.Headers[0]
			Expect(headermatch.Name).To(Equal("test"))
			regex := headermatch.GetRegexMatch()
			Expect(regex).To(Equal("testvalue"))
		})

	})

	Context("circuit breakers", func() {

		It("should NOT translate circuit breakers on upstream", func() {
			translate()
			Expect(cluster.CircuitBreakers).To(BeNil())
		})

		It("should translate circuit breakers on upstream", func() {

			upstream.UpstreamSpec.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 1},
				MaxPendingRequests: &types.UInt32Value{Value: 2},
				MaxRequests:        &types.UInt32Value{Value: 3},
				MaxRetries:         &types.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoycluster.CircuitBreakers{
				Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &types.UInt32Value{Value: 1},
						MaxPendingRequests: &types.UInt32Value{Value: 2},
						MaxRequests:        &types.UInt32Value{Value: 3},
						MaxRetries:         &types.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
		})

		It("should translate circuit breakers on settings", func() {

			settings.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 1},
				MaxPendingRequests: &types.UInt32Value{Value: 2},
				MaxRequests:        &types.UInt32Value{Value: 3},
				MaxRetries:         &types.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoycluster.CircuitBreakers{
				Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &types.UInt32Value{Value: 1},
						MaxPendingRequests: &types.UInt32Value{Value: 2},
						MaxRequests:        &types.UInt32Value{Value: 3},
						MaxRetries:         &types.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
		})

		It("should override circuit breakers on upstream", func() {

			settings.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 11},
				MaxPendingRequests: &types.UInt32Value{Value: 12},
				MaxRequests:        &types.UInt32Value{Value: 13},
				MaxRetries:         &types.UInt32Value{Value: 14},
			}

			upstream.UpstreamSpec.CircuitBreakers = &v1.CircuitBreakerConfig{
				MaxConnections:     &types.UInt32Value{Value: 1},
				MaxPendingRequests: &types.UInt32Value{Value: 2},
				MaxRequests:        &types.UInt32Value{Value: 3},
				MaxRetries:         &types.UInt32Value{Value: 4},
			}

			expectedCircuitBreakers := &envoycluster.CircuitBreakers{
				Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
					{
						MaxConnections:     &types.UInt32Value{Value: 1},
						MaxPendingRequests: &types.UInt32Value{Value: 2},
						MaxRequests:        &types.UInt32Value{Value: 3},
						MaxRetries:         &types.UInt32Value{Value: 4},
					},
				},
			}
			translate()

			Expect(cluster.CircuitBreakers).To(BeEquivalentTo(expectedCircuitBreakers))
		})

	})

	Context("when handling upstream groups", func() {

		var (
			upstream2     *v1.Upstream
			upstreamGroup *v1.UpstreamGroup
		)

		BeforeEach(func() {
			upstream2 = &v1.Upstream{
				Metadata: core.Metadata{
					Name:      "test2",
					Namespace: "gloo-system",
				},
				UpstreamSpec: &v1.UpstreamSpec{
					UpstreamType: &v1.UpstreamSpec_Static{
						Static: &v1static.UpstreamSpec{
							Hosts: []*v1static.Host{
								{
									Addr: "Test2",
									Port: 124,
								},
							},
						},
					},
				},
			}
			upstreamGroup = &v1.UpstreamGroup{
				Metadata: core.Metadata{
					Name:      "test",
					Namespace: "gloo-system",
				},
				Destinations: []*v1.WeightedDestination{
					{
						Weight: 1,
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
							},
						},
					},
					{
						Weight: 1,
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: utils.ResourceRefPtr(upstream2.Metadata.Ref()),
							},
						},
					},
				},
			}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, upstream2)
			params.Snapshot.UpstreamGroups = v1.UpstreamGroupList{
				upstreamGroup,
			}
			ref := upstreamGroup.Metadata.Ref()
			routes = []*v1.Route{{
				Matcher: matcher,
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_UpstreamGroup{
							UpstreamGroup: &ref,
						},
					},
				},
			}}
		})

		It("should translate upstream groups", func() {
			translate()

			route := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute()
			Expect(route).ToNot(BeNil())
			clusters := route.GetWeightedClusters()
			Expect(clusters).ToNot(BeNil())
			Expect(clusters.TotalWeight.Value).To(BeEquivalentTo(2))
			Expect(clusters.Clusters).To(HaveLen(2))
			Expect(clusters.Clusters[0].Name).To(Equal(UpstreamToClusterName(upstream.Metadata.Ref())))
			Expect(clusters.Clusters[1].Name).To(Equal(UpstreamToClusterName(upstream2.Metadata.Ref())))
		})

		It("should error on invalid ref in upstream groups", func() {
			upstreamGroup.Destinations[0].Destination.GetUpstream().Name = "notexist"

			_, errs, err := translator.Translate(params, proxy)
			Expect(err).NotTo(HaveOccurred())
			err = errs.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("destination # 1: upstream not found: list did not find upstream gloo-system.notexist"))
		})
	})

	Context("when handling subsets", func() {
		var (
			claConfiguration *envoyapi.ClusterLoadAssignment
		)
		BeforeEach(func() {
			claConfiguration = nil

			upstream.UpstreamSpec.UpstreamType = &v1.UpstreamSpec_Kube{
				Kube: &v1kubernetes.UpstreamSpec{
					SubsetSpec: &v1plugins.SubsetSpec{
						Selectors: []*v1plugins.Selector{{
							Keys: []string{
								"testkey",
							},
						}},
					},
				},
			}
			ref := upstream.Metadata.Ref()
			params.Snapshot.Endpoints = v1.EndpointList{
				{
					Metadata: core.Metadata{
						Name:      "test",
						Namespace: "gloo-system",
						Labels:    map[string]string{"testkey": "testvalue"},
					},
					Upstreams: []*core.ResourceRef{
						&ref,
					},
					Address: "1.2.3.4",
					Port:    1234,
				},
			}

			routes = []*v1.Route{{
				Matcher: matcher,
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
								},
								Subset: &v1.Subset{
									Values: map[string]string{
										"testkey": "testvalue",
									},
								},
							},
						},
					},
				},
			}}

		})

		translateWithEndpoints := func() {
			translate()

			endpoints := snapshot.GetResources(xds.EndpointType)

			clusterName := UpstreamToClusterName(upstream.Metadata.Ref())
			Expect(endpoints.Items).To(HaveKey(clusterName))
			endpointsResource := endpoints.Items[clusterName]
			claConfiguration = endpointsResource.ResourceProto().(*envoyapi.ClusterLoadAssignment)
			Expect(claConfiguration).NotTo(BeNil())
			Expect(claConfiguration.ClusterName).To(Equal(clusterName))
			Expect(claConfiguration.Endpoints).To(HaveLen(1))
			Expect(claConfiguration.Endpoints[0].LbEndpoints).To(HaveLen(len(params.Snapshot.Endpoints)))
		}

		Context("when happy path", func() {

			It("should transfer labels to envoy", func() {
				translateWithEndpoints()

				endpointMeta := claConfiguration.Endpoints[0].LbEndpoints[0].Metadata
				fields := endpointMeta.FilterMetadata["envoy.lb"].Fields
				Expect(fields).To(HaveKeyWithValue("testkey", sv("testvalue")))
			})

			It("should add subset to cluster", func() {
				translateWithEndpoints()

				Expect(cluster.LbSubsetConfig).ToNot(BeNil())
				Expect(cluster.LbSubsetConfig.FallbackPolicy).To(Equal(envoyapi.Cluster_LbSubsetConfig_ANY_ENDPOINT))
				Expect(cluster.LbSubsetConfig.SubsetSelectors).To(HaveLen(1))
				Expect(cluster.LbSubsetConfig.SubsetSelectors[0].Keys).To(HaveLen(1))
				Expect(cluster.LbSubsetConfig.SubsetSelectors[0].Keys[0]).To(Equal("testkey"))
			})
			It("should add subset to route", func() {
				translateWithEndpoints()

				metadatamatch := routeConfiguration.VirtualHosts[0].Routes[0].GetRoute().GetMetadataMatch()
				fields := metadatamatch.FilterMetadata["envoy.lb"].Fields
				Expect(fields).To(HaveKeyWithValue("testkey", sv("testvalue")))
			})
		})

		It("should create empty value if missing labels on the endpoint are provided in the upstream", func() {
			params.Snapshot.Endpoints[0].Metadata.Labels = nil
			translateWithEndpoints()
			endpointMeta := claConfiguration.Endpoints[0].LbEndpoints[0].Metadata
			Expect(endpointMeta).ToNot(BeNil())
			Expect(endpointMeta.FilterMetadata).To(HaveKey("envoy.lb"))
			fields := endpointMeta.FilterMetadata["envoy.lb"].Fields
			Expect(fields).To(HaveKeyWithValue("testkey", sv("")))
		})

		Context("bad route", func() {
			BeforeEach(func() {
				routes = []*v1.Route{{
					Matcher: matcher,
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_Single{
								Single: &v1.Destination{
									DestinationType: &v1.Destination_Upstream{
										Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
									},
									Subset: &v1.Subset{
										Values: map[string]string{
											"nottestkey": "value",
										},
									},
								},
							},
						},
					},
				}}
			})

			It("should error a route when subset in route doesnt match subset in upstream", func() {
				_, errs, err := translator.Translate(params, proxy)
				Expect(err).NotTo(HaveOccurred())
				Expect(errs.Validate()).To(HaveOccurred())
			})
		})
	})

	Context("when translating a route that points directly to a service", func() {

		var fakeUsList v1.UpstreamList

		BeforeEach(func() {

			// The kube service that we want to route to
			svc := skkube.NewService("ns-1", "svc-1")
			svc.Spec = k8scorev1.ServiceSpec{
				Ports: []k8scorev1.ServicePort{
					{
						Name:       "port-1",
						Port:       8080,
						TargetPort: intstr.FromInt(80),
					},
					{
						Name:       "port-2",
						Port:       8081,
						TargetPort: intstr.FromInt(8081),
					},
				},
			}
			// These are the "fake" upstreams that represent the above service in the snapshot
			fakeUsList = kubernetes.KubeServicesToUpstreams(skkube.ServiceList{svc})
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, fakeUsList...)

			// We need to manually add some fake endpoints for the above kubernetes services to the snapshot
			// Normally these would have been discovered by EDS
			params.Snapshot.Endpoints = v1.EndpointList{
				{
					Metadata: core.Metadata{
						Namespace: "gloo-system",
						Name:      fmt.Sprintf("ep-%v-%v", "192.168.0.1", svc.Spec.Ports[0].Port),
					},
					Port:      uint32(svc.Spec.Ports[0].Port),
					Address:   "192.168.0.1",
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: core.Metadata{
						Namespace: "gloo-system",
						Name:      fmt.Sprintf("ep-%v-%v", "192.168.0.2", svc.Spec.Ports[1].Port),
					},
					Port:      uint32(svc.Spec.Ports[1].Port),
					Address:   "192.168.0.2",
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[1].Metadata.Ref())},
				},
			}

			// Configure Proxy to route to the service
			serviceDestination := v1.Destination{
				DestinationType: &v1.Destination_Kube{
					Kube: &v1.KubernetesServiceDestination{
						Ref: core.ResourceRef{
							Namespace: svc.Namespace,
							Name:      svc.Name,
						},
						Port: uint32(svc.Spec.Ports[0].Port),
					},
				},
			}
			routes = []*v1.Route{{
				Matcher: matcher,
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &serviceDestination,
						},
					},
				},
			}}
		})

		It("generates the expected envoy route configuration", func() {
			translate()

			// Clusters have been created for the two "fake" upstreams
			clusters := snapshot.GetResources(xds.ClusterType)
			clusterResource := clusters.Items[UpstreamToClusterName(fakeUsList[0].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
			Expect(cluster).NotTo(BeNil())
			clusterResource = clusters.Items[UpstreamToClusterName(fakeUsList[1].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
			Expect(cluster).NotTo(BeNil())

			// A route to the kube service has been configured
			routes := snapshot.GetResources(xds.RouteType)
			Expect(routes.Items).To(HaveKey("listener-routes"))
			routeResource := routes.Items["listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoyrouteapi.Route_Route)
			Expect(ok).To(BeTrue())
			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoyrouteapi.RouteAction_Cluster)
			Expect(ok).To(BeTrue())
			Expect(clusterAction.Cluster).To(Equal(UpstreamToClusterName(fakeUsList[0].Metadata.Ref())))
		})
	})

	Context("when translating a route that points to a Consul service", func() {

		var (
			fakeUsList v1.UpstreamList
			dc         = func(dataCenterName string) string {
				return constants.ConsulDataCenterKeyPrefix + dataCenterName
			}
			tag = func(tagName string) string {
				return constants.ConsulTagKeyPrefix + tagName
			}

			trueValue = &types.Value{
				Kind: &types.Value_StringValue{
					StringValue: constants.ConsulEndpointMetadataMatchTrue,
				},
			}
			falseValue = &types.Value{
				Kind: &types.Value_StringValue{
					StringValue: constants.ConsulEndpointMetadataMatchFalse,
				},
			}
		)

		const (
			svcName = "my-consul-svc"

			// Data centers
			east = "east"
			west = "west"

			// Tags
			dev  = "dev"
			prod = "prod"

			yes = constants.ConsulEndpointMetadataMatchTrue
			no  = constants.ConsulEndpointMetadataMatchFalse
		)

		BeforeEach(func() {

			// Metadata for the Consul service that we want to route to
			svc := &consul.ServiceMeta{
				Name:        svcName,
				DataCenters: []string{east, west},
				Tags:        []string{dev, prod},
			}
			// These are the "fake" upstreams that represent the above service in the snapshot
			fakeUsList = v1.UpstreamList{consul.ToUpstream(svc)}
			params.Snapshot.Upstreams = append(params.Snapshot.Upstreams, fakeUsList...)

			// We need to manually add some fake endpoints for the above Consul service
			// Normally these would have been discovered by EDS
			params.Snapshot.Endpoints = v1.EndpointList{
				// 2 prod endpoints, 1 in each data center, 1 dev endpoint in west data center
				{
					Metadata: core.Metadata{
						Namespace: defaults.GlooSystem,
						Name:      svc.Name + "_1",
						Labels: map[string]string{
							dc(east):  yes,
							dc(west):  no,
							tag(dev):  no,
							tag(prod): yes,
						},
					},
					Port:      1001,
					Address:   "1.0.0.1",
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: core.Metadata{
						Namespace: defaults.GlooSystem,
						Name:      svc.Name + "_2",
						Labels: map[string]string{
							dc(east):  no,
							dc(west):  yes,
							tag(dev):  no,
							tag(prod): yes,
						},
					},
					Port:      2001,
					Address:   "2.0.0.1",
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[0].Metadata.Ref())},
				},
				{
					Metadata: core.Metadata{
						Namespace: defaults.GlooSystem,
						Name:      svc.Name + "_3",
						Labels: map[string]string{
							dc(east):  no,
							dc(west):  yes,
							tag(dev):  yes,
							tag(prod): no,
						},
					},
					Port:      2002,
					Address:   "2.0.0.2",
					Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(fakeUsList[0].Metadata.Ref())},
				},
			}

			// Configure Proxy to route to the service
			serviceDestination := v1.Destination{
				DestinationType: &v1.Destination_Consul{
					Consul: &v1.ConsulServiceDestination{
						ServiceName: svcName,
						Tags:        []string{prod},
						DataCenters: []string{east},
					},
				},
			}
			routes = []*v1.Route{{
				Matcher: matcher,
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &serviceDestination,
						},
					},
				},
			}}
		})

		It("generates the expected envoy route configuration", func() {
			translate()

			// A cluster has been created for the "fake" upstream and has the expected subset config
			clusters := snapshot.GetResources(xds.ClusterType)
			clusterResource := clusters.Items[UpstreamToClusterName(fakeUsList[0].Metadata.Ref())]
			cluster = clusterResource.ResourceProto().(*envoyapi.Cluster)
			Expect(cluster).NotTo(BeNil())
			Expect(cluster.LbSubsetConfig).NotTo(BeNil())
			Expect(cluster.LbSubsetConfig.SubsetSelectors).To(HaveLen(3))
			// Order is important here
			Expect(cluster.LbSubsetConfig.SubsetSelectors).To(ConsistOf(
				&envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{dc(east), dc(west)},
				},
				&envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{tag(dev), tag(prod)},
				},
				&envoyapi.Cluster_LbSubsetConfig_LbSubsetSelector{
					Keys: []string{dc(east), dc(west), tag(dev), tag(prod)},
				},
			))

			// A route to the kube service has been configured
			routes := snapshot.GetResources(xds.RouteType)
			Expect(routes.Items).To(HaveKey("listener-routes"))
			routeResource := routes.Items["listener-routes"]
			routeConfiguration = routeResource.ResourceProto().(*envoyapi.RouteConfiguration)
			Expect(routeConfiguration).NotTo(BeNil())
			Expect(routeConfiguration.VirtualHosts).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains).To(HaveLen(1))
			Expect(routeConfiguration.VirtualHosts[0].Domains[0]).To(Equal("*"))
			Expect(routeConfiguration.VirtualHosts[0].Routes).To(HaveLen(1))
			routeAction, ok := routeConfiguration.VirtualHosts[0].Routes[0].Action.(*envoyrouteapi.Route_Route)
			Expect(ok).To(BeTrue())

			clusterAction, ok := routeAction.Route.ClusterSpecifier.(*envoyrouteapi.RouteAction_Cluster)
			Expect(ok).To(BeTrue())
			Expect(clusterAction.Cluster).To(Equal(UpstreamToClusterName(fakeUsList[0].Metadata.Ref())))

			Expect(routeAction.Route).NotTo(BeNil())
			Expect(routeAction.Route.MetadataMatch).NotTo(BeNil())
			metadata, ok := routeAction.Route.MetadataMatch.FilterMetadata[EnvoyLb]
			Expect(ok).To(BeTrue())
			Expect(metadata.Fields).To(HaveLen(4))
			Expect(metadata.Fields[dc(east)]).To(Equal(trueValue))
			Expect(metadata.Fields[dc(west)]).To(Equal(falseValue))
			Expect(metadata.Fields[tag(dev)]).To(Equal(falseValue))
			Expect(metadata.Fields[tag(prod)]).To(Equal(trueValue))
		})
	})

	Context("Route plugin", func() {
		var (
			routePlugin *routePluginMock
		)
		BeforeEach(func() {
			routePlugin = &routePluginMock{}
			registeredPlugins = append(registeredPlugins, routePlugin)
		})

		It("should have the virtual host when processing route", func() {
			hasVhost := false
			routePlugin.ProcessRouteFunc = func(params plugins.RouteParams, in *v1.Route, out *envoyrouteapi.Route) error {
				if params.VirtualHost != nil {
					if params.VirtualHost.GetName() == "virt1" {
						hasVhost = true
					}
				}
				return nil
			}

			translate()
			Expect(hasVhost).To(BeTrue())
		})

	})

})

func sv(s string) *types.Value {
	return &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: s,
		},
	}
}

type routePluginMock struct {
	ProcessRouteFunc func(params plugins.RouteParams, in *v1.Route, out *envoyrouteapi.Route) error
}

func (p *routePluginMock) Init(params plugins.InitParams) error {
	return nil
}

func (p *routePluginMock) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyrouteapi.Route) error {
	return p.ProcessRouteFunc(params, in, out)
}
