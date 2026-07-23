package regexutils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/kgateway-dev/kgateway/v2/pkg/utils/regexutils"
)

var _ = Describe("Regex", func() {
	It("should create regex with default program size", func() {
		regex := NewRegexWithProgramSize("foo", nil)
		Expect(regex.GetRegex()).To(Equal("foo"))
		// GoogleRe2 is no longer set; Envoy defaults to RE2 engine
		Expect(regex.GetEngineType()).To(BeNil())
	})
	It("should create regex with a specific program size", func() {
		var number uint32
		number = 123
		// programsize is accepted for API compatibility but ignored (MaxProgramSize is deprecated by Envoy)
		regex := NewRegexWithProgramSize("foo", &number)
		Expect(regex.GetRegex()).To(Equal("foo"))
		// GoogleRe2 is no longer set; Envoy defaults to RE2 engine
		Expect(regex.GetEngineType()).To(BeNil())
	})
})
