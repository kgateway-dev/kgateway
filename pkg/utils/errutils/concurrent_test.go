package errutils

import (
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Concurrent", func() {
	Context("AggregateConcurrent", func() {
		It("returns the aggregate error", func() {
			badFns := []func() error{
				errorFn(fmt.Errorf("Erroring function. ID=%d", 1)),
				errorFn(fmt.Errorf("Erroring function. ID=%d", 2)),
			}

			aggregateErr := AggregateConcurrent(badFns)
			Expect(aggregateErr).To(HaveOccurred())
			Expect(aggregateErr.Errors()).To(ContainElements(
				MatchError(ContainSubstring("ID=1")),
				MatchError(ContainSubstring("ID=2")),
			))

		})

		It("executes all functions, even if 1 fails", func() {
			var sum = uint32(0)

			goodFns := []func() error{
				goodFn(&sum),
				goodFn(&sum),
				goodFn(&sum),
			}
			badFns := []func() error{
				errorFn(fmt.Errorf("Erroring function. ID=%d", 1)),
			}

			aggregateErr := AggregateConcurrent(append(badFns, goodFns...))
			Expect(aggregateErr).To(HaveOccurred())
			Expect(int(sum)).To(Equal(len(goodFns)), "all functions were executed")
		})
	})
})

func errorFn(err error) func() error {
	return func() error {
		return err
	}
}

func goodFn(sum *uint32) func() error {
	return func() error {
		atomic.AddUint32(sum, 1)
		return nil
	}
}
