package parallel_test

import (
	"github.com/solo-io/gloo/test/ginkgo/parallel"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ports", func() {

	Context("AdvancePortSafeDenylist", func() {

		It("skips ports in the denylist", func() {
			portInDenylist := uint32(10010)
			advanceAmount := uint32(1 + parallel.GetPortOffset())
			portInDenylistMinusOffset := portInDenylist - advanceAmount

			selectedPort := parallel.AdvancePortSafeDenylist(&portInDenylistMinusOffset)
			Expect(selectedPort).NotTo(Equal(portInDenylist))
			Expect(selectedPort).To(Equal(portInDenylist + 1))
		})

	})

})
