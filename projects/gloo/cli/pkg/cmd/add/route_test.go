package add_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Routes", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	BeforeEach(func() {
		err := testutils.Glooctl("create upstream static default-petstore-8080 --static-hosts jsonplaceholder.typicode.com:80")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should create static upstream", func() {
		err := testutils.Glooctl("add route --path-exact /sample-route-1 --dest-name default-petstore-8080 --prefix-rewrite /api/pets")
		Expect(err).NotTo(HaveOccurred())

		vs, err := helpers.MustVirtualServiceClient().Read("gloo-system", "default", clients.ReadOpts{})
		Expect(vs.Metadata.Name).To(Equal("default"))
	})

	It("should create static upstream group", func() {
		err := testutils.Glooctl("add route --path-exact /sample-route-1 --upstream-group-name petstore --upstream-group-namespace default --prefix-rewrite /api/pets")
		Expect(err).NotTo(HaveOccurred())

		vs, err := helpers.MustVirtualServiceClient().Read("gloo-system", "default", clients.ReadOpts{})
		Expect(vs.Metadata.Name).To(Equal("default"))
		ug := vs.VirtualHost.Routes[0].GetRouteAction().GetUpstreamGroup()
		Expect(ug.GetName()).To(Equal("petstore"))
		Expect(ug.GetNamespace()).To(Equal("default"))
	})

	It("should take in headers", func() {
		err := testutils.Glooctl("add route --path-exact /sample-route-a --dest-name default-petstore-8080 --header param1=value1,param2=,param3=")
		Expect(err).NotTo(HaveOccurred())

		vs, err := helpers.MustVirtualServiceClient().Read("gloo-system", "default", clients.ReadOpts{})
		Expect(vs.Metadata.Name).To(Equal("default"))
		parameters := vs.VirtualHost.Routes[0].Matcher.Headers
		Expect(parameters[0].Name).To(Equal("param1"))
		Expect(parameters[0].Value).To(Equal("value1"))
		Expect(parameters[1].Name).To(Equal("param2"))
		Expect(parameters[1].Value).To(Equal(""))
		Expect(parameters[2].Name).To(Equal("param3"))
		Expect(parameters[2].Value).To(Equal(""))
	})

	It("should take in query parameters", func() {
		err := testutils.Glooctl("add route --path-exact /sample-route-a --dest-name default-petstore-8080 --queryParameter param1=value1,param2=,param3=")
		Expect(err).NotTo(HaveOccurred())

		vs, err := helpers.MustVirtualServiceClient().Read("gloo-system", "default", clients.ReadOpts{})
		Expect(vs.Metadata.Name).To(Equal("default"))
		parameters := vs.VirtualHost.Routes[0].Matcher.QueryParameters
		Expect(parameters[0].Name).To(Equal("param1"))
		Expect(parameters[0].Value).To(Equal("value1"))
		Expect(parameters[1].Name).To(Equal("param2"))
		Expect(parameters[1].Value).To(Equal(""))
		Expect(parameters[2].Name).To(Equal("param3"))
		Expect(parameters[2].Value).To(Equal(""))
	})
})
