package gateway_test

import (
	"strings"

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
		Expect(strings.Contains(err.Error(), "unknown shorthand flag")).To(BeFalse())
	})

	It("should allow -l and -p flags after proxy url", func() {
		err := testutils.Glooctl("proxy address -l -p test")
		Expect(strings.Contains(err.Error(), "unknown shorthand flag")).To(BeFalse())
	})
})
