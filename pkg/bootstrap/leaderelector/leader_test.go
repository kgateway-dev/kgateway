package leaderelector_test

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
)

var _ = Describe("Leader Startup Action", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		electedChan chan struct{}
		identity    leaderelector.Identity

		startupAction *leaderelector.LeaderStartupAction
		startupOps    uint64
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		electedChan = make(chan struct{})
		identity = leaderelector.NewIdentity(electedChan)

		startupAction = leaderelector.NewLeaderStartupAction(identity)
		startupAction.SetAction(func() error {
			atomic.AddUint64(&startupOps, 1)
			return nil
		})
		startupOps = 0
	})

	AfterEach(func() {
		select {
		case <-electedChan:
			// channel is closed, do nothing
		default:
			// channel is still open, close it
			close(electedChan)
		}

		cancel()
	})

	It("StartupAction not executed, if not elected", func() {
		startupAction.WatchElectionResults(ctx)

		Consistently(func(g Gomega) {
			g.Expect(atomic.LoadUint64(&startupOps)).To(Equal(uint64(0)))
		}).ShouldNot(HaveOccurred())
	})

	It("StartupAction executed once, if elected", func() {
		startupAction.WatchElectionResults(ctx)

		// signal election
		close(electedChan)

		Eventually(func(g Gomega) {
			g.Expect(atomic.LoadUint64(&startupOps)).To(Equal(uint64(1)))
		}).ShouldNot(HaveOccurred())
		Consistently(func(g Gomega) {
			g.Expect(atomic.LoadUint64(&startupOps)).To(Equal(uint64(1)))
		}).ShouldNot(HaveOccurred())

	})

	It("StartupAction not executed after context cancelled", func() {
		startupAction.WatchElectionResults(ctx)

		// Cancelling the context should stop the election watch
		cancel()

		Consistently(func(g Gomega) {
			g.Expect(atomic.LoadUint64(&startupOps)).To(Equal(uint64(0)))
		}).ShouldNot(HaveOccurred())

		// signal election
		close(electedChan)

		Consistently(func(g Gomega) {
			g.Expect(atomic.LoadUint64(&startupOps)).To(Equal(uint64(0)))
		}).ShouldNot(HaveOccurred())

	})
})
