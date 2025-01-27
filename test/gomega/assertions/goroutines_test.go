//go:build ignore

package assertions_test

import (
	"context"

	"github.com/kgateway-dev/kgateway/test/gomega/assertions"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gleak"
	"github.com/onsi/gomega/types"
)

var _ = Describe("GoRoutineMonitor", func() {

	It("succeeds when there are no new go routines", func() {
		monitor := assertions.NewGoRoutineMonitor()
		monitor.ExpectNoLeaks(&assertions.ExpectNoLeaksArgs{})
	})

	It("fails when there are new go routines", func() {
		monitor := assertions.NewGoRoutineMonitor()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-ctx.Done()
		}()

		failures := InterceptGomegaFailures(func() { monitor.ExpectNoLeaks(&assertions.ExpectNoLeaksArgs{}) })
		Expect(failures).NotTo(BeEmpty())
	})

	It("succeeds when there are new go routines that we expected", func() {
		monitor := assertions.NewGoRoutineMonitor()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-ctx.Done()
		}()

		monitor.ExpectNoLeaks(&assertions.ExpectNoLeaksArgs{
			AllowedRoutines: []types.GomegaMatcher{
				gleak.IgnoringInBacktrace("github.com/kgateway-dev/kgateway/test/gomega/assertions_test.init"),
			},
		})

	})

})
