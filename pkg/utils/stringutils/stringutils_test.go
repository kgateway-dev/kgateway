package stringutils_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/utils/stringutils"
)

var _ = Describe("StringUtils", func() {
	DescribeTable("DeleteOneByValue", func(array []string, value string, expected []string) {
		Expect(DeleteOneByValue(array, value)).To(Equal(expected))
	},
		Entry("Empty", []string{}, "", []string{}),
		Entry("Empty Array", []string{}, "one", []string{}),
		Entry("Empty Value", []string{"one", "two", "three"}, "", []string{"one", "two", "three"}),
		Entry("basic one", []string{"one", "two", "three"}, "one", []string{"two", "three"}),
		Entry("basic two", []string{"one", "two", "three"}, "two", []string{"one", "three"}),
		Entry("basic three", []string{"one", "two", "three"}, "three", []string{"one", "two"}),
		Entry("Only once", []string{"one", "two", "one", "three"}, "one", []string{"two", "one", "three"}),
		Entry("Not Found", []string{"one", "two", "three"}, "four", []string{"one", "two", "three"}),
	)

})
