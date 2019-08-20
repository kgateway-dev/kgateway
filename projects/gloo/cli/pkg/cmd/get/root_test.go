package get_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/get"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Root", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	Context("Empty args and flags", func() {
		It("should give clear error message", func() {
			// Ignore the output message since it changes whenever we add flags and it is tested via the cobra lib.
			_, err := testutils.GlooctlOut("get")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(get.EmptyGetError.Error()))
		})
	})
})
