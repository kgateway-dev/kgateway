package create_test

import (
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
)

func Glooctl(args string) error {
	app := cmd.GlooCli("test")
	app.SetArgs(strings.Split(args, " "))
	return app.Execute()
}

var _ = Describe("Upstream", func() {

	BeforeSuite(func() {
		helpers.MemoryResourceClient = &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
	})

	It("should create static upstream", func() {
		err := Glooctl("create upstream static jsonplaceholder-80 --static-hosts jsonplaceholder.typicode.com:80")
		Expect(err).NotTo(HaveOccurred())

		up, err := helpers.MustUpstreamClient().Read("default", "jsonplaceholder-80", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(up.Metadata.Name).To(Equal("jsonplaceholder-80"))
	})
})
