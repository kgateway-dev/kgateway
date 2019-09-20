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
		output, err := testutils.GlooctlOut("proxy url -l -p test")
		if err != nil {
			Expect(output).To(ContainSubstring("host does not exist, unable to show an IP"))
		}
	})

	It("should allow -l and -p flags after proxy url", func() {
		output, err := testutils.GlooctlOut("proxy address -l -p test")
		if err != nil {
			Expect(output).To(ContainSubstring("host does not exist, unable to show an IP"))
		}
	})
})
