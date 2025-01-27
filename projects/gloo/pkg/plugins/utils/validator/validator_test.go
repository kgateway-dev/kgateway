package validator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/statsutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"go.opencensus.io/stats/view"
)

// Validation against envoy can not be tested here as it is designed to pass if the envoy binary is not found. So we test the cache and metrics instead.
// Ref: https://github.com/solo-io/gloo/blob/7e503dea039fa69211232c83bd07f8c169df0d45/projects/gloo/pkg/bootstrap/bootstrap_validation.go#L53
// Validation is tested in the enterprise gateway kube2e tests
var _ = Describe("Validator", func() {

	var ctx context.Context
	var testProto = &transformation.TransformationStages{
		InheritTransformation: false,
	}

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("Updates the stats", func() {
		cacheHitsName := "gloo.solo.io/test_validation_cache_hits"
		cacheMissesName := "gloo.solo.io/test_validation_cache_misses"
		mCacheHits := statsutils.MakeSumCounter(cacheHitsName, "The number of cache hits while validating test config")
		mCacheMisses := statsutils.MakeSumCounter(cacheMissesName, "The number of cache misses while validating test config")

		validator := New("test", "test",
			WithCounters(mCacheHits, mCacheMisses))
		Expect(validator.CacheLength()).To(Equal(0))

		validator.ValidateConfig(ctx, testProto)

		// On the first validation, the cacheMiss = 1, cacheHits = empty, cache length = 1
		rows, err := view.RetrieveData(cacheMissesName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.SumData).Value).To(Equal(float64(1)))
		rows, err = view.RetrieveData(cacheHitsName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(BeEmpty())
		Expect(validator.CacheLength()).To(Equal(1))

		validator.ValidateConfig(ctx, testProto)

		// On the next validation of the same proto, the cacheMiss = 1, cacheHits = 1, cache length = 1
		rows, err = view.RetrieveData(cacheMissesName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.SumData).Value).To(Equal(float64(1)))
		rows, err = view.RetrieveData(cacheHitsName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.SumData).Value).To(Equal(float64(1)))
		Expect(validator.CacheLength()).To(Equal(1))

		testProto.InheritTransformation = true
		validator.ValidateConfig(ctx, testProto)

		// On the validation of another proto, the cacheMiss = 2, cacheHits = 1, cache length = 2
		rows, err = view.RetrieveData(cacheMissesName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.SumData).Value).To(Equal(float64(2)))
		rows, err = view.RetrieveData(cacheHitsName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.SumData).Value).To(Equal(float64(1)))
		Expect(validator.CacheLength()).To(Equal(2))
	})

	It("creates its own counters", func() {
		validator := New("custom", "custom")

		cacheHitsName := "gloo.solo.io/custom_validation_cache_hits"
		cacheMissesName := "gloo.solo.io/custom_validation_cache_misses"

		// validate twice to populate both caches
		validator.ValidateConfig(ctx, testProto)
		validator.ValidateConfig(ctx, testProto)

		// On validation of the same proto twice, the cacheMiss = 1, cacheHits = 1
		rows, err := view.RetrieveData(cacheMissesName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.SumData).Value).To(Equal(float64(1)))
		rows, err = view.RetrieveData(cacheHitsName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.SumData).Value).To(Equal(float64(1)))
	})

})
