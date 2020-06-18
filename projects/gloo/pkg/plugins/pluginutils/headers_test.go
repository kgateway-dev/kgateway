package pluginutils_test

import (
	"context"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var _ = Describe("Headers", func() {
	var (
		in      *v1.Route
		out     *envoyroute.Route
		header  *envoycore.HeaderValueOption
		headers []*envoycore.HeaderValueOption
	)
	BeforeEach(func() {
		header = &envoycore.HeaderValueOption{
			Header: &envoycore.HeaderValue{
				Key:   "test",
				Value: "header",
			},
		}
		headers = []*envoycore.HeaderValueOption{header}
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
			out = &envoyroute.Route{
				Action: &envoyroute.Route_Route{
					Route: &envoyroute.RouteAction{
						ClusterSpecifier: &envoyroute.RouteAction_Cluster{
							Cluster: "test",
						},
					},
				},
			}
		})

		It("should add header config to upstream", func() {

			err := MarkHeaders(context.TODO(), &v1.ApiSnapshot{}, in, out, func(spec *v1.Destination) ([]*envoycore.HeaderValueOption, error) {
				return headers, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.RequestHeadersToAdd).To(ContainElement(header))
		})
		It("should not removing existing headers", func() {
			existingheader := &envoycore.HeaderValueOption{
				Header: &envoycore.HeaderValue{
					Key:   "test1",
					Value: "header1",
				},
			}
			out.RequestHeadersToAdd = append(out.RequestHeadersToAdd, existingheader)
			err := MarkHeaders(context.TODO(), &v1.ApiSnapshot{}, in, out, func(spec *v1.Destination) ([]*envoycore.HeaderValueOption, error) {
				return headers, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.RequestHeadersToAdd).To(ContainElement(existingheader))
			Expect(out.RequestHeadersToAdd).To(ContainElement(header))
		})
	})

	Context("multiple dests", func() {
		var (
			yescluster *envoyroute.WeightedCluster_ClusterWeight
			nocluster  *envoyroute.WeightedCluster_ClusterWeight
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

			yescluster = &envoyroute.WeightedCluster_ClusterWeight{
				Name: "yes",
			}
			nocluster = &envoyroute.WeightedCluster_ClusterWeight{
				Name: "no",
			}
			out = &envoyroute.Route{
				Action: &envoyroute.Route_Route{
					Route: &envoyroute.RouteAction{
						ClusterSpecifier: &envoyroute.RouteAction_WeightedClusters{
							WeightedClusters: &envoyroute.WeightedCluster{
								Clusters: []*envoyroute.WeightedCluster_ClusterWeight{yescluster, nocluster},
							},
						},
					},
				},
			}
		})
		It("should add per filter config only to relevant upstream in mutiple dest", func() {

			err := MarkHeaders(context.TODO(), &v1.ApiSnapshot{}, in, out, func(spec *v1.Destination) ([]*envoycore.HeaderValueOption, error) {
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
				snap *v1.ApiSnapshot
			)
			BeforeEach(func() {
				upGrp := &v1.UpstreamGroup{
					Metadata: core.Metadata{
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
								UpstreamGroup: &ref,
							},
						},
					},
				}
				snap = &v1.ApiSnapshot{
					UpstreamGroups: v1.UpstreamGroupList{
						upGrp,
					},
				}

			})

			It("should add per filter config only to relevant upstream in mutiple dest", func() {

				err := MarkHeaders(context.TODO(), snap, in, out, func(spec *v1.Destination) ([]*envoycore.HeaderValueOption, error) {
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
