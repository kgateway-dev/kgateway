package translator_test

import (
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var _ = FDescribe("Route Configs", func() {
	DescribeTable("validate route path", func(path string, expectedValue bool) {
		Expect(translator.ValidateRoutePath(path)).To(Equal(expectedValue))
	},
		Entry("Hex", "%af", true),
		Entry("Hex Camel", "%Af", true),
		Entry("Hex num", "%00", true),
		Entry("Hex double", "%11", true),
		Entry("Hex with valid", "%af801&*", true),
		Entry("valid with hex", "801&*%af", true),
		Entry("valid with hex and valid", "801&*%af719$@!", true),
		Entry("Hex single", "%0", false),
		Entry("unicode chars", "ƒ©", false),
		Entry("unicode chars", "¥¨˚∫", false),
	)

	It("Should validate all seperate characters", func() {
		// must allow all "pchar" characters = unreserved / pct-encoded / sub-delims / ":" / "@"
		// https://www.rfc-editor.org/rfc/rfc3986
		// unreserved
		// alpha Upper and Lower
		for i := 'a'; i <= 'z'; i++ {
			Expect(translator.ValidateRoutePath(string(i))).To(Equal(true))
			Expect(translator.ValidateRoutePath(strings.ToUpper(string(i)))).To(Equal(true))
		}
		// digit
		for i := 0; i < 10; i++ {
			Expect(translator.ValidateRoutePath(strconv.Itoa(i))).To(Equal(true))
		}
		unreservedChars := "-._~"
		for _, c := range unreservedChars {
			Expect(translator.ValidateRoutePath(string(c))).To(Equal(true))
		}
		// sub-delims
		subDelims := "!$&'()*+,;="
		Expect(len(subDelims)).To(Equal(11))
		for _, c := range subDelims {
			Expect(translator.ValidateRoutePath(string(c))).To(Equal(true))
		}
		// pchar
		pchar := ":@"
		for _, c := range pchar {
			Expect(translator.ValidateRoutePath(string(c))).To(Equal(true))
		}
		// invalid characters
		invalid := "<>?/\\|[]{}\"^%#"
		for _, c := range invalid {
			Expect(translator.ValidateRoutePath(string(c))).To(Equal(false))
		}
	})

})
