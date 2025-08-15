package assertions_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gleak"
	"github.com/onsi/gomega/types"

	"github.com/kgateway-dev/kgateway/v2/test/gomega/assertions"
)

var _ = Describe("GoRoutineMonitor", func() {

	It("succeeds when there are no new go routines", func() {
		monitor := assertions.NewGoRoutineMonitor()
		monitor.AssertNoLeaks(&assertions.AssertNoLeaksArgs{})
	})

	It("fails when there are new go routines", func() {
		monitor := assertions.NewGoRoutineMonitor()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-ctx.Done()
		}()

		failures := InterceptGomegaFailures(func() { monitor.AssertNoLeaks(&assertions.AssertNoLeaksArgs{}) })
		Expect(failures).NotTo(BeEmpty())
	})

	It("succeeds when there are new go routines that we expected", func() {
		monitor := assertions.NewGoRoutineMonitor()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-ctx.Done()
		}()

		monitor.AssertNoLeaks(&assertions.AssertNoLeaksArgs{
			AllowedRoutines: []types.GomegaMatcher{
				gleak.IgnoringInBacktrace("github.com/kgateway-dev/kgateway/v2/test/gomega/assertions_test.init"),
			},
		})

	})

})
