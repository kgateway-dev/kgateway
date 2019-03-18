package pluginutils_test

import (
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
)

var _ = Describe("ClusterExtensions", func() {

	var (
		out  *envoyapi.Cluster
		msg  *types.Struct
		name string
	)
	BeforeEach(func() {
		msg = &types.Struct{
			Fields: map[string]*types.Value{
				"test": &types.Value{Kind: &types.Value_BoolValue{
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
			Expect(out.ExtensionProtocolOptions).To(HaveKeyWithValue(name, BeEquivalentTo(msg)))
		})
	})

})
