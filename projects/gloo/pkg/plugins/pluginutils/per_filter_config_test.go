package pluginutils_test

import (
	"context"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("TypedPerFilterConfig", func() {
	var (
		in      *v1.Route
		out     *envoy_config_route_v3.Route
		msg     *structpb.Struct
		message *any.Any
		name    string
	)
	BeforeEach(func() {
		msg = &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"test": {Kind: &structpb.Value_BoolValue{
					BoolValue: true,
				}},
			},
		}
		var err error
		message, err = utils.MessageToAny(msg)
		Expect(err).NotTo(HaveOccurred())
		name = "fakename"

	})
	Context("set typed per filter config", func() {
		BeforeEach(func() {
			out = &envoy_config_route_v3.Route{}
		})

		It("should add typed per filter config to route", func() {
			err := SetRoutePerFilterConfig(out, name, msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(name, message))
		})
		It("should add typed per filter config to vhost", func() {
			out := &envoy_config_route_v3.VirtualHost{}
			err := SetVhostPerFilterConfig(out, name, msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(name, message))
		})
		It("should add typed per filter config to cluster weight", func() {
			out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
			err := SetWeightedClusterPerFilterConfig(out, name, msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(name, message))
		})
	})

	Context("single dests", func() {

		BeforeEach(func() {
			in = &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: &core.ResourceRef{
										Name:      "test",
										Namespace: "",
									},
								},
							},
						},
					},
				},
			}
			out = &envoy_config_route_v3.Route{
				Action: &envoy_config_route_v3.Route_Route{
					Route: &envoy_config_route_v3.RouteAction{
						ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
							Cluster: "test",
						},
					},
				},
			}
		})

		It("should add typed per filter config to upstream", func() {

			err := MarkPerFilterConfig(context.TODO(), &v1snap.ApiSnapshot{}, in, out, name, func(spec *v1.Destination) (proto.Message, error) {
				return msg, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(name, message))
		})

		It("should add typed per filter config only to relevant upstream", func() {

			err := MarkPerFilterConfig(context.TODO(), &v1snap.ApiSnapshot{}, in, out, name, func(spec *v1.Destination) (proto.Message, error) {
				return nil, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedPerFilterConfig).ToNot(HaveKey(name))
		})
	})
	Context("multiple dests", func() {
		var (
			yescluster *envoy_config_route_v3.WeightedCluster_ClusterWeight
			nocluster  *envoy_config_route_v3.WeightedCluster_ClusterWeight
		)

		BeforeEach(func() {
			in = &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Multi{
							Multi: &v1.MultiDestination{
								Destinations: []*v1.WeightedDestination{{
									Destination: &v1.Destination{
										DestinationType: &v1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "yes",
												Namespace: "",
											},
										},
									},
								}, {
									Destination: &v1.Destination{
										DestinationType: &v1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "no",
												Namespace: "",
											},
										},
									},
								}},
							},
						},
					},
				},
			}

			yescluster = &envoy_config_route_v3.WeightedCluster_ClusterWeight{
				Name: "yes",
			}
			nocluster = &envoy_config_route_v3.WeightedCluster_ClusterWeight{
				Name: "no",
			}
			out = &envoy_config_route_v3.Route{
				Action: &envoy_config_route_v3.Route_Route{
					Route: &envoy_config_route_v3.RouteAction{
						ClusterSpecifier: &envoy_config_route_v3.RouteAction_WeightedClusters{
							WeightedClusters: &envoy_config_route_v3.WeightedCluster{
								Clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{yescluster, nocluster},
							},
						},
					},
				},
			}
		})

		It("should add typed per filter config only to relevant upstream in mutiple dest", func() {

			err := MarkPerFilterConfig(context.TODO(), &v1snap.ApiSnapshot{}, in, out, name, func(spec *v1.Destination) (proto.Message, error) {
				if spec.GetUpstream().Name == "yes" {
					return msg, nil
				}
				return nil, nil
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(yescluster.TypedPerFilterConfig).To(HaveKeyWithValue(name, message))
			Expect(nocluster.TypedPerFilterConfig).ToNot(HaveKey(name))
			Expect(out.TypedPerFilterConfig).ToNot(HaveKey(name))

		})
		Context("upstream group", func() {
			var (
				snap *v1snap.ApiSnapshot
			)
			BeforeEach(func() {
				upGrp := &v1.UpstreamGroup{
					Metadata: &core.Metadata{
						Name:      "test",
						Namespace: "test",
					},
					Destinations: []*v1.WeightedDestination{{
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      "yes",
									Namespace: "",
								},
							},
						},
					}, {
						Destination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      "no",
									Namespace: "",
								},
							},
						}},
					},
				}
				ref := upGrp.Metadata.Ref()
				in = &v1.Route{
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_UpstreamGroup{
								UpstreamGroup: ref,
							},
						},
					},
				}
				snap = &v1snap.ApiSnapshot{
					UpstreamGroups: v1.UpstreamGroupList{
						upGrp,
					},
				}

			})

			It("should add typed per filter config only to relevant upstream in mutiple dest", func() {

				err := MarkPerFilterConfig(context.TODO(), snap, in, out, name, func(spec *v1.Destination) (proto.Message, error) {
					if spec.GetUpstream().Name == "yes" {
						return msg, nil
					}
					return nil, nil
				})
				Expect(err).NotTo(HaveOccurred())

				Expect(yescluster.TypedPerFilterConfig).To(HaveKeyWithValue(name, message))
				Expect(nocluster.TypedPerFilterConfig).ToNot(HaveKey(name))
				Expect(out.TypedPerFilterConfig).ToNot(HaveKey(name))

			})
		})
	})
	Context("nil destination", func() {
		BeforeEach(func() {
			in = &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{},
				},
			}
			out = &envoy_config_route_v3.Route{
				Action: &envoy_config_route_v3.Route_Route{
					Route: &envoy_config_route_v3.RouteAction{
						ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
							Cluster: "test",
						},
					},
				},
			}
		})
		It("should not throw NPE when destination is nil", func() {
			err := MarkPerFilterConfig(context.TODO(), &v1snap.ApiSnapshot{}, in, out, name, func(spec *v1.Destination) (proto.Message, error) {
				return nil, nil
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unexpected destination type that is nil"))
		})
	})
})
