package gateway_test

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Debug", func() {
	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	It("should allow -l and -p flags after proxy url", func() {
		err := testutils.Glooctl("proxy url -l -p test")
		Expect(err).To(SatisfyAny(BeNil(), MatchError("host does not exist, unable to show an IP")))
	})

	It("should allow -l and -p flags after proxy url", func() {
		err := testutils.Glooctl("proxy address -l -p test")
		Expect(err).To(SatisfyAny(BeNil(), MatchError("host does not exist, unable to show an IP")))
	})
})
