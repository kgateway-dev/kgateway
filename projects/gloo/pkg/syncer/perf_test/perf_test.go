package perf_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Envoy Translator Syncer Performance Tests", Ordered, func() {

	// We have future work to introduce some benchmarking tests in this space
	// By introducing a failing test first, we can prove out a mechanism to not run these
	// tests on PRs, and only run them during nightly tests
	// Once we get a failure on our nightly tests, we know that these tests are being reported properly
	It("Smoke Test (Intentional Failure)", func() {
		Expect(1).To(Equal(2), "Intentionally failing test")
	})

})
