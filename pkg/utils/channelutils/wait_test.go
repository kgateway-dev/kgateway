package channelutils_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/v2/pkg/utils/channelutils"
)

var _ = Describe("Wait", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})
	AfterEach(func() {
		cancel()
	})

	Context("WaitForReady", func() {
		It("should timeout when channels don't warm up", func() {
			timeout := time.Millisecond
			err := WaitForReady(ctx, timeout, make(chan struct{}))
			Expect(err).To(MatchError(context.DeadlineExceeded))
		})
		It("should succeed when channels are ready", func() {
			timeout := time.Millisecond
			closedChannels0, closedChannels1 := make(chan struct{}), make(chan struct{})
			close(closedChannels0)
			close(closedChannels1)
			err := WaitForReady(ctx, timeout, closedChannels0, closedChannels1)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should exit when context is canceled", func() {
			timeout := time.Second
			go func() {
				time.Sleep(time.Second / 3)
				cancel()
			}()
			err := WaitForReady(ctx, timeout, make(chan struct{}))
			Expect(err).To(MatchError(context.Canceled))
		})
	})
})
