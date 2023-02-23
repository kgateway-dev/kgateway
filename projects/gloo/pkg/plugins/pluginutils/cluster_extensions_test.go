package pluginutils_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var _ = Describe("ClusterExtensions", func() {

	var (
		out  *envoy_config_cluster_v3.Cluster
		msg  *structpb.Struct
		name string
	)
	BeforeEach(func() {
		msg = &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"test": {Kind: &structpb.Value_BoolValue{
					BoolValue: true,
				}},
			},
		}
		name = "fakename"

	})
	Context("set per filter config", func() {
		BeforeEach(func() {
			out = &envoy_config_cluster_v3.Cluster{}
		})

		It("should add per filter config to route", func() {
			err := SetExtensionProtocolOptions(out, name, msg)
			Expect(err).NotTo(HaveOccurred())
			anyMsg, err := utils.MessageToAny(msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedExtensionProtocolOptions).To(HaveKeyWithValue(name, BeEquivalentTo(anyMsg)))
		})
	})

})
