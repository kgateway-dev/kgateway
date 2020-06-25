package pluginutils_test

import (
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
)

var _ = Describe("ClusterExtensions", func() {

	var (
		out  *envoyapi.Cluster
		msg  *structpb.Struct
		name string
	)
	BeforeEach(func() {
		msg = &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"test": &structpb.Value{Kind: &structpb.Value_BoolValue{
					BoolValue: true,
				}},
			},
		}
		name = "fakename"

	})
	Context("set per filter config", func() {
		BeforeEach(func() {
			out = &envoyapi.Cluster{}
		})

		It("should add per filter config to route", func() {
			err := SetExtenstionProtocolOptions(out, name, msg)
			Expect(err).NotTo(HaveOccurred())
			anyMsg, err := MessageToAny(msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedExtensionProtocolOptions).To(HaveKeyWithValue(name, BeEquivalentTo(anyMsg)))
		})
	})

})
