package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
)

var _ = Describe("PathAsString", func() {
	It("returns the correct string regardless of the path matcher proto type", func() {
		Expect(PathAsString(&matchers.Matcher{
			PathSpecifier: &matchers.Matcher_Exact{"hi"},
		})).To(Equal("hi"))
		Expect(PathAsString(&matchers.Matcher{
			PathSpecifier: &matchers.Matcher_Prefix{"hi"},
		})).To(Equal("hi"))
		Expect(PathAsString(&matchers.Matcher{
			PathSpecifier: &matchers.Matcher_Regex{"howsitgoin"},
		})).To(Equal("howsitgoin"))
	})
	It("returns empty string for empty matcher", func() {
		Expect(PathAsString(&matchers.Matcher{})).To(Equal(""))
		Expect(PathAsString(nil)).To(Equal(""))
	})
})
