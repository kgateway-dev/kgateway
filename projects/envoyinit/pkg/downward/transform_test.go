package downward_test

import (
	envoy_config_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/envoyinit/pkg/downward"
)

var _ = Describe("Transform", func() {

	Context("bootstrap transforms", func() {
		var (
			api             *mockDownward
			bootstrapConfig *envoy_config_bootstrap.Bootstrap
		)
		BeforeEach(func() {
			api = &mockDownward{
				podName: "Test",
			}
			bootstrapConfig = new(envoy_config_bootstrap.Bootstrap)
			bootstrapConfig.Node = &envoy_core.Node{}
		})

		It("should transform node id", func() {

			bootstrapConfig.Node.Id = "{{.PodName}}"
			err := TransformConfigTemplatesWithApi(bootstrapConfig.Node, api)
			Expect(err).NotTo(HaveOccurred())
			Expect(bootstrapConfig.Node.Id).To(Equal("Test"))
		})

		It("should transform cluster", func() {
			bootstrapConfig.Node.Cluster = "{{.PodName}}"
			err := TransformConfigTemplatesWithApi(bootstrapConfig.Node, api)
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

			err := TransformConfigTemplatesWithApi(bootstrapConfig.Node, api)
			Expect(err).NotTo(HaveOccurred())
			Expect(bootstrapConfig.Node.Metadata.Fields["foo"].Kind.(*structpb.Value_StringValue).StringValue).To(Equal("Test"))
		})
	})
})
