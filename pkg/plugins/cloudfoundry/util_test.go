package cloudfoundry_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/plugins/cloudfoundry"
)

var _ = Describe("Util", func() {
	It("Resync", func() {

		counter := 0

		waitforRsync := make(chan struct{})
		resyncfunc := func() { counter += 1; waitforRsync <- struct{}{} }

		ticker := make(chan time.Time, 1)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go ResyncLoopWithTicker(ctx, resyncfunc, ticker, nil)
		// Make sure we receive only once (the first resync)
		Eventually(waitforRsync).Should(Receive())
		Eventually(waitforRsync).ShouldNot(Receive())

		Expect(counter).To(BeEquivalentTo(1))
		ticker <- time.Now()

		// Make sure we receive only once (after the tick)
		Eventually(waitforRsync).Should(Receive())
		Eventually(waitforRsync).ShouldNot(Receive())

		Expect(counter).To(BeEquivalentTo(2))
	})

	It("stop channel cancels the channel", func() {
		ctx := context.Background()
		stop := make(chan struct{})
		ctx, cancel := MakeStopCancelContext(ctx, stop)
		defer cancel()
		Eventually(ctx.Done()).ShouldNot(Receive())
		close(stop)
		Eventually(ctx.Done()).Should(BeClosed())

	})

	It("should trigger and update when kicker", func() {

		counter := 0

		waitforRsync := make(chan struct{})
		resyncfunc := func() { counter += 1; waitforRsync <- struct{}{} }

		ticker := make(chan time.Time, 1)
		kickchan := make(chan struct{}, 1)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go ResyncLoopWithTicker(ctx, resyncfunc, ticker, kickchan)
		// clear the first resync in the loop
		Eventually(waitforRsync).Should(Receive())

		// kick the loop
		kickchan <- struct{}{}

		Eventually(waitforRsync).Should(Receive())
		Expect(counter).To(BeEquivalentTo(2))

	})
})
