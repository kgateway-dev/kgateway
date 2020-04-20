package listener_test

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/listener"
)

var _ = Describe("Plugin", func() {

	var (
		plugin *Plugin
		out    *envoyapi.Listener
	)
	BeforeEach(func() {
		out = new(envoyapi.Listener)
		plugin = NewPlugin()
	})

	It("should set HealthyPanicThreshold", func() {

		in := &v1.Listener{
			Options: &v1.ListenerOptions{
				PerConnectionBufferLimitBytes: &types.UInt32Value{
					Value: uint32(4096),
				},
			},
		}
		err := plugin.ProcessListener(plugins.Params{}, in, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PerConnectionBufferLimitBytes.Value).To(BeEquivalentTo(uint32(4096)))
	})
})
