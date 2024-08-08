package sliceutils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/utils/sliceutils"
)

var _ = Describe("SliceUtils", func() {
	DescribeTable("Dedupe strings", func(in, expected []string) {
		Expect(Dedupe(in)).To(Equal(expected))
	},
		Entry("Empty", []string{}, []string{}),
		Entry("No dupes", []string{"one", "two", "three"}, []string{"one", "two", "three"}),
		Entry("One dupe", []string{"one", "two", "one", "three"}, []string{"one", "two", "three"}),
		Entry("Multiple elements duped", []string{"one", "two", "one", "two", "three"}, []string{"one", "two", "three"}),
		Entry("One element duped multiple times", []string{"one", "two", "one", "three", "one"}, []string{"one", "two", "three"}),
		Entry("Multiple elements duped multiple times", []string{"one", "two", "one", "two", "three", "one", "three"}, []string{"one", "two", "three"}),
	)

	DescribeTable("Dedupe ints", func(in, expected []int) {
		Expect(Dedupe(in)).To(Equal(expected))
	},
		Entry("Empty", []int{}, []int{}),
		Entry("No dupes", []int{1, 2, 3}, []int{1, 2, 3}),
		Entry("One dupe", []int{1, 2, 1, 3}, []int{1, 2, 3}),
		Entry("Multiple elements duped", []int{1, 2, 1, 2, 3}, []int{1, 2, 3}),
		Entry("One element duped multiple times", []int{1, 2, 1, 3, 1}, []int{1, 2, 3}),
		Entry("Multiple elements duped multiple times", []int{1, 2, 1, 2, 3, 1, 3}, []int{1, 2, 3}),
	)
})
