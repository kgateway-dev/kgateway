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

	Describe("HTTPFilters", func() {
		It("has Gzip filter when gzip is enabled", func() {
			plug := &Plugin{}
			route := NewTestRouteWithGzip()
			out := &envoyroute.Route{
				Action: &envoyroute.Route_Route{},
			}
			err := plug.ProcessRoute(nil, route, out)
			Expect(err).NotTo(HaveOccurred())
			filters := plug.HttpFilters(nil)
			Expect(len(filters)).To(Equal(1))
			Expect(filters[0].HttpFilter.Name).To(Equal("envoy.gzip"))
		})

		It("does not contain Gzip filter by default", func() {
			plug := &Plugin{}
			route := NewTestRouteWithCORS()
			out := &envoyroute.Route{
				Action: &envoyroute.Route_Route{},
			}
			err := plug.ProcessRoute(nil, route, out)
			Expect(err).NotTo(HaveOccurred())
			filters := plug.HttpFilters(nil)
			for _, f := range filters {
				Expect(f.HttpFilter.Name).NotTo(Equal("envoy.gzip"))
			}
		})
	})
})
