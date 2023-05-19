package linkerd

import (
	"fmt"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("linkerd plugin", func() {

	var (
		params plugins.Params
		plugin plugins.RoutePlugin
		out    *envoy_config_route_v3.Route
	)

	BeforeEach(func() {
		out = new(envoy_config_route_v3.Route)

		params = plugins.Params{}
		plugin = NewPlugin()
	})

	Context("create header for upstream", func() {
		It("returns the proper envoy header object", func() {
			ns := "namespace"
			name := "name"
			var port uint32 = 7000
			kus := &kubernetes.UpstreamSpec{
				ServicePort:      port,
				ServiceNamespace: ns,
				ServiceName:      name,
			}
			host := fmt.Sprintf("%s.%s.svc.cluster.local:%v", name, ns, port)
			Expect(createHeaderForUpstream(kus)).To(matchers.MatchProto(
				&envoy_config_core_v3.HeaderValueOption{
					Header: &envoy_config_core_v3.HeaderValue{
						Value: host,
						Key:   HeaderKey,
					},
					Append: &wrappers.BoolValue{
						Value: false,
					},
				},
			))
		})
	})

	var createUpstream = func(ref *core.ResourceRef, spec *kubernetes.UpstreamSpec) *v1.Upstream {
		upstream := &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      ref.Name,
				Namespace: ref.Namespace,
			},
		}
		upstream.UpstreamType = &v1.Upstream_Static{
			Static: &static.UpstreamSpec{},
		}
		if spec != nil {
			upstream.UpstreamType = &v1.Upstream_Kube{
				Kube: spec,
			}
		}
		return upstream
	}

	var clustersAndDestinationsForUpstreams = func(
		upstreamRefs []*core.ResourceRef,
	) ([]*envoy_config_route_v3.WeightedCluster_ClusterWeight, []*v1.WeightedDestination) {
		clusters := make([]*envoy_config_route_v3.WeightedCluster_ClusterWeight, len(upstreamRefs))
		for i, v := range upstreamRefs {
			clusters[i] = &envoy_config_route_v3.WeightedCluster_ClusterWeight{
				Name: translator.UpstreamToClusterName(v),
			}
		}
		destinations := make([]*v1.WeightedDestination, len(upstreamRefs))
		for i, v := range upstreamRefs {
			usRef := v
			destinations[i] = &v1.WeightedDestination{
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: usRef,
					},
				},
			}
		}
		return clusters, destinations
	}

	var createUpstreamList = func(refs []*core.ResourceRef, specs []*kubernetes.UpstreamSpec) v1.UpstreamList {
		upstreams := make(v1.UpstreamList, len(refs))
		for i, v := range refs {
			upstreams[i] = createUpstream(v, specs[i])
		}
		return upstreams
	}

	Context("config for multi destination", func() {
		It("doesn't change output if route action doesn't exist", func() {
			out.Action = &envoy_config_route_v3.Route_DirectResponse{DirectResponse: &envoy_config_route_v3.DirectResponseAction{}}
			outCopy := proto.Clone(out)
			Expect(configForMultiDestination(nil, nil, out)).To(BeNil())
			Expect(out).To(matchers.MatchProto(outCopy))
		})
		It("doesn't change output if route action exists, but weighted clusters do not", func() {
			out.Action = &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
						Cluster: "",
					},
				},
			}
			outCopy := proto.Clone(out)
			Expect(configForMultiDestination(nil, nil, out)).To(BeNil())
			Expect(out).To(matchers.MatchProto(outCopy))
		})
		It("does not change output if no kube upstreams exist", func() {
			out.Action = &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_config_route_v3.WeightedCluster{},
					},
				},
			}
			usRf := &core.ResourceRef{
				Name:      "one",
				Namespace: "two",
			}
			destinations := &v1.WeightedDestination{
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: usRf,
					},
				},
			}
			upstreams := createUpstream(usRf, nil)
			outCopy := proto.Clone(out)
			Expect(configForMultiDestination([]*v1.WeightedDestination{destinations}, v1.UpstreamList{upstreams}, out)).To(BeNil())
			Expect(out).To(matchers.MatchProto(outCopy))
		})
		It("properly adds the header to existing weighted clusters with kube upstreams", func() {
			upstreamRefs := []*core.ResourceRef{
				{Name: "one", Namespace: "one"},
				{Name: "two", Namespace: "one"},
				{Name: "three", Namespace: "one"},
			}
			var port uint32 = 9000
			kubeSpecs := []*kubernetes.UpstreamSpec{
				{ServicePort: port, ServiceName: "one", ServiceNamespace: "one"},
				{ServicePort: port, ServiceName: "two", ServiceNamespace: "one"},
				{ServicePort: port, ServiceName: "three", ServiceNamespace: "one"},
			}
			clusters, destinations := clustersAndDestinationsForUpstreams(upstreamRefs)
			out.Action = &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_config_route_v3.WeightedCluster{
							Clusters: clusters,
						},
					},
				},
			}
			upstreams := createUpstreamList(upstreamRefs, kubeSpecs)
			outCopy := proto.Clone(out)
			Expect(configForMultiDestination(destinations, upstreams, out)).To(BeNil())
			Expect(out).NotTo(BeEquivalentTo(outCopy))
			routeAction := out.GetRoute()
			Expect(routeAction).NotTo(BeNil())
			weightedClusters := routeAction.GetWeightedClusters()
			Expect(weightedClusters).NotTo(BeNil())
			Expect(weightedClusters.Clusters).To(HaveLen(3))
			for i, v := range kubeSpecs {
				Expect(weightedClusters.Clusters[i].RequestHeadersToAdd).To(ContainElement(createHeaderForUpstream(v)))
			}

		})
		It("skips non-kubernetes upstreams", func() {
			upstreamRefs := []*core.ResourceRef{
				{Name: "one", Namespace: "one"},
				{Name: "two", Namespace: "one"},
				{Name: "three", Namespace: "one"},
			}
			var port uint32 = 9000
			kubeSpecs := []*kubernetes.UpstreamSpec{
				{ServicePort: port, ServiceName: "one", ServiceNamespace: "one"},
				{ServicePort: port, ServiceName: "two", ServiceNamespace: "one"},
				nil,
			}
			clusters, destinations := clustersAndDestinationsForUpstreams(upstreamRefs)
			out.Action = &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_config_route_v3.WeightedCluster{
							Clusters: clusters,
						},
					},
				},
			}
			upstreams := createUpstreamList(upstreamRefs, kubeSpecs)
			outCopy := proto.Clone(out)
			Expect(configForMultiDestination(destinations, upstreams, out)).To(BeNil())
			Expect(out).NotTo(BeEquivalentTo(outCopy))
			routeAction := out.GetRoute()
			Expect(routeAction).NotTo(BeNil())
			weightedClusters := routeAction.GetWeightedClusters()
			Expect(weightedClusters).NotTo(BeNil())
			Expect(weightedClusters.Clusters).To(HaveLen(3))
			for i, v := range kubeSpecs {
				if v != nil {
					Expect(weightedClusters.Clusters[i].RequestHeadersToAdd).To(ContainElement(createHeaderForUpstream(v)))
				} else {
					Expect(weightedClusters.Clusters[i].RequestHeadersToAdd).To(BeNil())
				}
			}
		})
	})

	Context("through the plugin", func() {

		It("works for a single", func() {
			upstreamRefs := []*core.ResourceRef{
				{Name: "one", Namespace: "one"},
				{Name: "two", Namespace: "one"},
				{Name: "three", Namespace: "one"},
			}
			var port uint32 = 9000
			kubeSpecs := []*kubernetes.UpstreamSpec{
				{ServicePort: port, ServiceName: "one", ServiceNamespace: "one"},
				{ServicePort: port, ServiceName: "two", ServiceNamespace: "one"},
				nil,
			}
			clusters, _ := clustersAndDestinationsForUpstreams(upstreamRefs)
			out.Action = &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_WeightedClusters{
						WeightedClusters: &envoy_config_route_v3.WeightedCluster{
							Clusters: clusters,
						},
					},
				},
			}
			upstreams := createUpstreamList(upstreamRefs, kubeSpecs)
			params.Snapshot = &v1snap.ApiSnapshot{
				Upstreams: upstreams,
			}
			in := &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: upstreamRefs[0],
								},
							},
						},
					},
				},
			}
			outCopy := proto.Clone(out)
			plugin.Init(plugins.InitParams{
				Settings: &v1.Settings{
					Linkerd: true,
				},
			})

			err := plugin.ProcessRoute(plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{Params: params}}, in, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).NotTo(BeEquivalentTo(outCopy))
			Expect(out.RequestHeadersToAdd).To(ContainElement(createHeaderForUpstream(kubeSpecs[0])))
		})
	})
})
