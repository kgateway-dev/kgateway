package pluginutils_test

import (
	"context"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var _ = Describe("Headers", func() {
	var (
		in      *v1.Route
		out     *envoy_config_route_v3.Route
		header  *envoy_config_core_v3.HeaderValueOption
		headers []*envoy_config_core_v3.HeaderValueOption
	)
	BeforeEach(func() {
		header = &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   "test",
				Value: "header",
			},
		}
		headers = []*envoy_config_core_v3.HeaderValueOption{header}
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

		It("should add header config to upstream", func() {

			err := MarkHeaders(context.TODO(), &v1snap.ApiSnapshot{}, in, out, func(spec *v1.Destination) ([]*envoy_config_core_v3.HeaderValueOption, error) {
				return headers, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.RequestHeadersToAdd).To(ContainElement(header))
		})
		It("should not removing existing headers", func() {
			existingheader := &envoy_config_core_v3.HeaderValueOption{
				Header: &envoy_config_core_v3.HeaderValue{
					Key:   "test1",
					Value: "header1",
				},
			}
			out.RequestHeadersToAdd = append(out.RequestHeadersToAdd, existingheader)
			err := MarkHeaders(context.TODO(), &v1snap.ApiSnapshot{}, in, out, func(spec *v1.Destination) ([]*envoy_config_core_v3.HeaderValueOption, error) {
				return headers, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.RequestHeadersToAdd).To(ContainElement(existingheader))
			Expect(out.RequestHeadersToAdd).To(ContainElement(header))
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
		It("should add per filter config only to relevant upstream in mutiple dest", func() {

			err := MarkHeaders(context.TODO(), &v1snap.ApiSnapshot{}, in, out, func(spec *v1.Destination) ([]*envoy_config_core_v3.HeaderValueOption, error) {
				if spec.GetUpstream().Name == "yes" {
					return headers, nil
				}
				return nil, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(yescluster.RequestHeadersToAdd).To(ContainElement(header))
			Expect(nocluster.RequestHeadersToAdd).NotTo(ContainElement(header))
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
						},
					}},
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

			It("should add per filter config only to relevant upstream in mutiple dest", func() {

				err := MarkHeaders(context.TODO(), snap, in, out, func(spec *v1.Destination) ([]*envoy_config_core_v3.HeaderValueOption, error) {
					if spec.GetUpstream().Name == "yes" {
						return headers, nil
					}
					return nil, nil
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(yescluster.RequestHeadersToAdd).To(ContainElement(header))
				Expect(nocluster.RequestHeadersToAdd).NotTo(ContainElement(header))
			})

		})

	})
})
