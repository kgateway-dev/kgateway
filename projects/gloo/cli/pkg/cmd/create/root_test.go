package create_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Root", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	Context("Empty args and flags", func() {
		It("should give clear error message", func() {
			err := testutils.Glooctl("create")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(create.EmptyCreateError))
		})
	})
})
