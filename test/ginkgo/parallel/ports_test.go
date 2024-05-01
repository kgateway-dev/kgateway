package parallel_test

import (
	"github.com/avast/retry-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/ginkgo/parallel"
)

var _ = Describe("Ports", func() {

	Context("AdvancePortSafe", func() {

		portInUseDenylist := func(proposedPort uint32) error {
			var denyList = map[uint32]struct{}{
				10010: {}, // used by Gloo, when devMode is enabled
				10011: {}, // We include a few extra ports to ensure that retry logic works as expected
				10012: {},
			}

			if _, ok := denyList[proposedPort]; ok {
				return eris.Errorf("port %d is in use", proposedPort)
			}
			return nil
		}

		It("skips ports based on errIfPortInUse", func() {
			portInDenylist := uint32(10010)
			advanceAmount := uint32(1 + parallel.GetPortOffset())
			portInDenylistMinusOffset := portInDenylist - advanceAmount

			selectedPort, err := parallel.AdvancePortSafe(&portInDenylistMinusOffset, portInUseDenylist, retry.Delay(0))
			Expect(err).NotTo(HaveOccurred())
			Expect([]uint32{10010, 10011, 10012}).NotTo(ContainElement(selectedPort), "should have skipped the ports in the denylist")
			Expect(selectedPort).To(Equal(uint32(10013)), "should have selected the next port")
		})

		It("exhausts all retries, and returns last attempt and error, if retry was unsuccessful", func() {
			startingPort := uint32(10010)
			selectedPort, err := parallel.AdvancePortSafe(&startingPort, func(proposedPort uint32) error {
				// We always error here, to ensure that we continue to retry advancing the port
				return eris.Errorf("Port invalid: %d", proposedPort)
			}, retry.Delay(0))
			Expect(err).To(HaveOccurred())
			Expect(selectedPort).To(Equal(uint32(11015)), "should have exhausted 5 retries")
		})

	})

})
