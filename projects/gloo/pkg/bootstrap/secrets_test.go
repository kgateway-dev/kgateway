package bootstrap_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
)

var _ = FDescribe("secrets", func() {
	Context("multi client factory", func() {
		It("returns an error when a nil source map is provided", func() {
			_, err := NewMultiSecretSourceResourceClientFactory(nil, nil, nil, nil, nil, nil)
			Expect(err).To(MatchError(ErrNilSourceSlice))
		})
	})
})
