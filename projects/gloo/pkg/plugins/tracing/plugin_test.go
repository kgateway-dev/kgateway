package tracing

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {

	It("should update routes properly", func() {
		p := NewPlugin()
		in := &v1.Route{}
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, in, out)
		Expect(err).NotTo(HaveOccurred())

		inFull := &v1.Route{
			Matcher: nil,
			Action:  nil,
			RoutePlugins: &v1.RoutePlugins{
				Tracing: &hcm.RouteTracingSettings{
					RouteDescriptor: "hello",
				},
			},
		}
		outFull := &envoyroute.Route{}
		err = p.ProcessRoute(plugins.RouteParams{}, inFull, outFull)
		Expect(err).NotTo(HaveOccurred())
		Expect(outFull.Decorator.Operation).To(Equal("hello"))
		Expect(outFull.Tracing.ClientSampling.Numerator).To(Equal(uint32(100)))
		Expect(outFull.Tracing.RandomSampling.Numerator).To(Equal(uint32(0)))
		Expect(outFull.Tracing.OverallSampling.Numerator).To(Equal(uint32(100)))
	})

})
