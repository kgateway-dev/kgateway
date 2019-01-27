package create_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
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

		staticSpec := up.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Static).Static
		expectedHosts := []*static.Host{{Addr: "jsonplaceholder.typicode.com", Port: 80}}
		Expect(staticSpec.Hosts).To(Equal(expectedHosts))
	})

	It("should create aws upstream", func() {
		err := testutils.Glooctl("create upstream aws --aws-region us-east-1 --aws-secret-name aws-lambda-access --name aws-us-east-1")
		Expect(err).NotTo(HaveOccurred())

		up, err := helpers.MustUpstreamClient().Read("gloo-system", "aws-us-east-1", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		awsspec := up.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Aws).Aws
		Expect(awsspec.Region).To(Equal("us-east-1"))
		Expect(awsspec.SecretRef.Name).To(Equal("aws-lambda-access"))

	})
})
