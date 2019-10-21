package utils

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("PathAsString", func() {
	It("returns the correct string regardless of the path matcher proto type", func() {
		Expect(PathAsString(&v1.Matcher{
			PathSpecifier: &v1.Matcher_Exact{"hi"},
		})).To(Equal("hi"))
		Expect(PathAsString(&v1.Matcher{
			PathSpecifier: &v1.Matcher_Prefix{"hi"},
		})).To(Equal("hi"))
		Expect(PathAsString(&v1.Matcher{
			PathSpecifier: &v1.Matcher_Regex{"howsitgoin"},
		})).To(Equal("howsitgoin"))
	})
	It("returns empty string for empty matcher", func() {
		Expect(PathAsString(&v1.Matcher{})).To(Equal(""))
		Expect(PathAsString(nil)).To(Equal(""))
	})
})
