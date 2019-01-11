package timeout_test

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/timeout"
	"time"
)

var _ = Describe("Plugin", func() {
	It("works", func() {
		t := time.Minute
		p := NewPlugin()
		routeAction := &envoyroute.RouteAction{}
		out := &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: routeAction,
			},
		}
		err := p.ProcessRoute(plugins.Params{}, &v1.Route{
			RoutePlugins: &v1.RoutePlugins{
				Timeout: types.DurationProto(t),
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeAction.Timeout).NotTo(BeNil())
		Expect(*routeAction.Timeout).To(Equal(t))
	})
})
