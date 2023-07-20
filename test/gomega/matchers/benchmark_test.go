package matchers_test

import (
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/gomega/matchers"
)

var _ = Describe("Benchmark", func() {

	// Note that logic for determining percentile is tested in test/helpers/util_test.go
	Describe("Percentile matchers", func() {
		DescribeTable("HavePercentileLessThan",
			func(percentile int, upperBound time.Duration, size int, shouldMatch bool) {
				durations := durationSlice(size)

				if shouldMatch {
					Expect(durations).To(matchers.HavePercentileLessThan(percentile, upperBound))
				} else {
					Expect(durations).NotTo(matchers.HavePercentileLessThan(percentile, upperBound))
				}
			},
			Entry("duration at percentile < target", 80, 5*time.Second, 5, true),
			Entry("duration at percentile = target", 80, 4*time.Second, 5, false),
			Entry("duration at percentile > target", 80, 3*time.Second, 5, false),
		)

		DescribeTable("HavePercentileWithin",
			func(percentile int, upperBound, window time.Duration, size int, shouldMatch bool) {
				durations := durationSlice(size)

				if shouldMatch {
					Expect(durations).To(matchers.HavePercentileWithin(percentile, upperBound, window))
				} else {
					Expect(durations).NotTo(matchers.HavePercentileWithin(percentile, upperBound, window))
				}
			},
			Entry("duration at percentile < target, below window", 80, 10*time.Second, time.Second, 10, false),
			Entry("duration at percentile < target, within window", 80, 9*time.Second, time.Second, 10, true),
			Entry("duration at percentile = target", 80, 8*time.Second, time.Second, 10, true),
			Entry("duration at percentile > target, within window", 80, 7*time.Second, time.Second, 10, true),
			Entry("duration at percentile > target, above window", 80, 6*time.Second, time.Second, 10, false),
		)
	})

	DescribeTable("HaveMedianLessThan",
		func(upperBound time.Duration, size int, shouldMatch bool) {
			durations := durationSlice(size)

			if shouldMatch {
				Expect(durations).To(matchers.HaveMedianLessThan(upperBound))
			} else {
				Expect(durations).NotTo(matchers.HaveMedianLessThan(upperBound))
			}
		},
		Entry("odd length, median < target", 6*time.Second, 9, true),
		Entry("odd length, median = target", 5*time.Second, 9, false),
		Entry("odd length, median > target", 4*time.Second, 9, false),
		Entry("even length, median < target", 6*time.Second, 10, true),
		Entry("even length, median = target", 5500*time.Millisecond, 10, false),
		Entry("even length, median > target", 5*time.Second, 10, false),
	)
})

// durationSlice returns a slice with durations of 1s, 2s, ..., ns for n = size in a randomized order
func durationSlice(size int) []time.Duration {
	durations := make([]time.Duration, size)
	for i := time.Duration(0); i < time.Duration(size); i++ {
		durations[i] = (i * time.Second) + time.Second
	}
	rand.Shuffle(size, func(i, j int) { durations[i], durations[j] = durations[j], durations[i] })

	return durations
}
