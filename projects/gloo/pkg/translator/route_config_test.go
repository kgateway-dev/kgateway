package translator_test

import (
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var _ = Describe("Route Configs", func() {

	DescribeTable("validate route path", func(path string, expectedValue bool) {
		if expectedValue {
			Expect(translator.ValidateRoutePath(path)).ToNot(HaveOccurred())
		} else {
			Expect(translator.ValidateRoutePath(path)).To(HaveOccurred())
		}
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
		Entry("//", "hello/something//", false),
		Entry("/./", "hello/something/./", false),
		Entry("/../", "hello/something/../", false),
		Entry("hex slash upper", "hello/something%2F", false),
		Entry("hex slash lower", "hello/something%2f", false),
		Entry("hash", "hello/something#", false),
		Entry("/..", "hello/../something", false),
		Entry("/.", "hello/./something", false),
	)

	It("Should validate all separate characters", func() {
		// must allow all "pchar" characters = unreserved / pct-encoded / sub-delims / ":" / "@"
		// https://www.rfc-editor.org/rfc/rfc3986
		// unreserved
		// alpha Upper and Lower
		for i := 'a'; i <= 'z'; i++ {
			Expect(translator.ValidateRoutePath(string(i))).ToNot(HaveOccurred())
			Expect(translator.ValidateRoutePath(strings.ToUpper(string(i)))).ToNot(HaveOccurred())
		}
		// digit
		for i := 0; i < 10; i++ {
			Expect(translator.ValidateRoutePath(strconv.Itoa(i))).ToNot(HaveOccurred())
		}
		unreservedChars := "-._~"
		for _, c := range unreservedChars {
			Expect(translator.ValidateRoutePath(string(c))).ToNot(HaveOccurred())
		}
		// sub-delims
		subDelims := "!$&'()*+,;="
		Expect(len(subDelims)).To(Equal(11))
		for _, c := range subDelims {
			Expect(translator.ValidateRoutePath(string(c))).ToNot(HaveOccurred())
		}
		// pchar
		pchar := ":@"
		for _, c := range pchar {
			Expect(translator.ValidateRoutePath(string(c))).ToNot(HaveOccurred())
		}
		// invalid characters
		invalid := "<>?\\|[]{}\"^%#"
		for _, c := range invalid {
			Expect(translator.ValidateRoutePath(string(c))).To(HaveOccurred())
		}
	})

	DescribeTable("path rewrites", func(s string, pass bool) {
		err := translator.ValidatePrefixRewrite(s)
		if pass {
			Expect(err).ToNot(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
		}
	},
		Entry("allow query parameters", "some/site?a=data&b=location&c=searchterm", true),
		Entry("allow fragments", "some/site#framgentedinfo", true),
		Entry("invalid", "some/site<hello", false),
		Entry("invalid", "some/site{hello", false),
		Entry("invalid", "some/site}hello", false),
		Entry("invalid", "some/site[hello", false),
	)
})
