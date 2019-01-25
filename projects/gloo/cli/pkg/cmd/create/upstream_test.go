package create_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Upstream", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	It("should create static upstream", func() {
		err := testutils.Glooctl("create upstream static jsonplaceholder-80 --static-hosts jsonplaceholder.typicode.com:80")
		Expect(err).NotTo(HaveOccurred())

		up, err := helpers.MustUpstreamClient().Read("gloo-system", "jsonplaceholder-80", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(up.Metadata.Name).To(Equal("jsonplaceholder-80"))
	})
})
