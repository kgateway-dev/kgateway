package downward_test

import (
	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/structpb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/downward"
)

var _ = Describe("Transform", func() {

	Context("bootstrap transforms", func() {
		var (
			api             *mockDownward
			bootstrapConfig *envoybootstrapv3.Bootstrap
		)
		BeforeEach(func() {
			api = &mockDownward{
				podName: "Test",
				nodeIp:  "5.5.5.5",
			}
			bootstrapConfig = new(envoybootstrapv3.Bootstrap)
			bootstrapConfig.Node = &envoycorev3.Node{}
		})

		It("should transform node id", func() {

			bootstrapConfig.Node.Id = "{{.PodName}}"
			err := TransformConfigTemplatesWithApi(bootstrapConfig, api)
			Expect(err).NotTo(HaveOccurred())
			Expect(bootstrapConfig.Node.Id).To(Equal("Test"))
		})

		It("should transform cluster", func() {
			bootstrapConfig.Node.Cluster = "{{.PodName}}"
			err := TransformConfigTemplatesWithApi(bootstrapConfig, api)
			Expect(err).NotTo(HaveOccurred())
			Expect(bootstrapConfig.Node.Cluster).To(Equal("Test"))
		})

		It("should transform metadata", func() {
			bootstrapConfig.Node.Metadata = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"foo": {
						Kind: &structpb.Value_StringValue{
							StringValue: "{{.PodName}}",
						},
					},
				},
			}

			err := TransformConfigTemplatesWithApi(bootstrapConfig, api)
			Expect(err).NotTo(HaveOccurred())
			Expect(bootstrapConfig.Node.Metadata.Fields["foo"].Kind.(*structpb.Value_StringValue).StringValue).To(Equal("Test"))
		})

		It("should transform static resources", func() {
			bootstrapConfig.StaticResources = &envoybootstrapv3.Bootstrap_StaticResources{
				Clusters: []*envoyclusterv3.Cluster{{
					LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
						Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{
							LbEndpoints: []*envoyendpointv3.LbEndpoint{{
								HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
									Endpoint: &envoyendpointv3.Endpoint{
										Address: &envoycorev3.Address{
											Address: &envoycorev3.Address_SocketAddress{
												SocketAddress: &envoycorev3.SocketAddress{
													Address: "{{.NodeIp}}",
												},
											},
										},
									},
								},
							}},
						}},
					},
				}},
			}

			err := TransformConfigTemplatesWithApi(bootstrapConfig, api)
			Expect(err).NotTo(HaveOccurred())

			expectedAddress := bootstrapConfig.GetStaticResources().GetClusters()[0].GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress()
			Expect(expectedAddress).To(Equal("5.5.5.5"))
		})

	})
})
