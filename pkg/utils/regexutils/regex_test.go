package regexutils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/pkg/utils/regexutils"
)

var _ = Describe("Regex", func() {
	It("should create regex with default program size", func() {
		regex := NewRegexWithProgramSize("foo", nil)
		Expect(regex.GetRegex()).To(Equal("foo"))

		regex = NewRegex(nil, "foo")
		Expect(regex.GetRegex()).To(Equal("foo"))
		Expect(regex.GetGoogleRe2().GetMaxProgramSize()).To(BeNil())
	})
	It("should create regex with a specific program size", func() {
		var number uint32
		number = 123
		regex := NewRegexWithProgramSize("foo", &number)
		Expect(regex.GetRegex()).To(Equal("foo"))
		Expect(regex.GetGoogleRe2().GetMaxProgramSize().GetValue()).To(Equal(number))
	})

})
