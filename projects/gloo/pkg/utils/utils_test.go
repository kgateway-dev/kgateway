package utils_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/utils"
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
	It("panics if an invalid matcher is passed", func() {
		Expect(func() { PathAsString(&v1.Matcher{}) }).To(Panic())
		Expect(func() { PathAsString(nil) }).To(Panic())
	})
})
