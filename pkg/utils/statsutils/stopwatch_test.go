package statsutils_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	statsutils "github.com/kgateway-dev/kgateway/v2/pkg/utils/statsutils"
)

var _ = Describe("StopWatch", func() {
	var (
		sw      statsutils.StopWatch
		measure *stats.Float64Measure
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		measure = stats.Float64("test_measure", "Test measurement", stats.UnitMilliseconds)
		ctx = context.Background()

		key, err := tag.NewKey("test_key")
		Expect(err).NotTo(HaveOccurred())

		sw = statsutils.NewStopWatch(measure, tag.Insert(key, "test_value"))
	})

	It("should measure elapsed time correctly", func() {
		sw.Start()
		time.Sleep(100 * time.Millisecond)
		duration := sw.Stop(ctx)

		Expect(duration).To(BeNumerically("~", 100*time.Millisecond, 50*time.Millisecond))
	})

	It("should handle multiple measurements", func() {
		// First measurement
		sw.Start()
		time.Sleep(50 * time.Millisecond)
		duration1 := sw.Stop(ctx)
		Expect(duration1).To(BeNumerically("~", 50*time.Millisecond, 30*time.Millisecond))

		// Second measurement
		sw.Start()
		time.Sleep(100 * time.Millisecond)
		duration2 := sw.Stop(ctx)
		Expect(duration2).To(BeNumerically("~", 100*time.Millisecond, 30*time.Millisecond))
	})

	It("should work with zero duration", func() {
		sw.Start()
		duration := sw.Stop(ctx)
		Expect(duration).To(BeNumerically("<", 10*time.Millisecond))
	})
})
