package leaderelector_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
)

var _ = Describe("Identity", func() {

	var (
		electedChan chan struct{}
		identity    leaderelector.Identity
	)

	BeforeEach(func() {
		electedChan = make(chan struct{})
		identity = leaderelector.NewIdentity(electedChan)
	})

	AfterEach(func() {
		select {
		case <-electedChan:
			// channel is closed, do nothing
		default:
			// channel is still open, close it
			close(electedChan)
		}
	})

	It("IsLeader always returns false, then true after channel is closed", func() {
		Consistently(func(g Gomega) {
			g.Expect(identity.IsLeader()).To(BeFalse())
		}).ShouldNot(HaveOccurred())

		close(electedChan)

		Consistently(func(g Gomega) {
			g.Expect(identity.IsLeader()).To(BeTrue())
		}).ShouldNot(HaveOccurred())
	})

})
