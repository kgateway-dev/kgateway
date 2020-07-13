package protocoloptions_test

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/protocoloptions"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/types"
)

var _ = Describe("Plugin", func() {

	var (
		p      *protocoloptions.Plugin
		params plugins.Params
		out    *envoyapi.Cluster
	)

	BeforeEach(func() {
		p = protocoloptions.NewPlugin()
		out = new(envoyapi.Cluster)

	})
	Context("upstream", func() {
		It("should not accept connection streams that are too small", func() {
			tooSmall := &v1.Upstream{
				InitialConnectionWindowSize: &types.UInt32Value{Value: 65534},
			}

			err := p.ProcessUpstream(params, tooSmall, out)
			Expect(err).To(HaveOccurred())
		})
		It("should not accept connection streams that are too large", func() {
			tooBig := &v1.Upstream{
				InitialStreamWindowSize: &types.UInt32Value{Value: 2147483648},
			}
			err := p.ProcessUpstream(params, tooBig, out)
			Expect(err).To(HaveOccurred())
		})
		It("should accept connection streams that are within the correct range", func() {
			validUpstream := &v1.Upstream{
				InitialStreamWindowSize:     &types.UInt32Value{Value: 268435457},
				InitialConnectionWindowSize: &types.UInt32Value{Value: 65535},
			}

			err := p.ProcessUpstream(params, validUpstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Http2ProtocolOptions).NotTo(BeNil())
			Expect(out.Http2ProtocolOptions.InitialStreamWindowSize).To(Equal(&wrappers.UInt32Value{Value: 268435457}))
			Expect(out.Http2ProtocolOptions.InitialConnectionWindowSize).To(Equal(&wrappers.UInt32Value{Value: 65535}))
		})
	})
})
