package extensions_test

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/coreplugins/route-extensions"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Plugin", func() {
	Describe("ProcessRoute", func() {
		It("takes CORS policy generates cors for envoy", func() {
			plug := &Plugin{}
			route := NewTestRouteWithCORS()
			out := &envoyroute.Route{
				Action: &envoyroute.Route_Route{},
			}
			err := plug.ProcessRoute(nil, route, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.GetRoute()).NotTo(BeNil())
			Expect(out.GetRoute().Cors).NotTo(BeNil())
			Expect(out.GetRoute().Cors.AllowMethods).To(Equal("GET, POST"))
			Expect(out.GetRoute().Cors.AllowOrigin).To(ContainElement("*.solo.io"))
			Expect(out.GetRoute().Cors.MaxAge).To(Equal("86400"))
		})
	})
})
