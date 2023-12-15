package utils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var _ = Describe("utils.PathAsString", func() {
	It("returns the correct string regardless of the path matcher proto type", func() {
		Expect(utils.PathAsString(&matchers.Matcher{
			PathSpecifier: &matchers.Matcher_Exact{"hi"},
		})).To(Equal("hi"))
		Expect(utils.PathAsString(&matchers.Matcher{
			PathSpecifier: &matchers.Matcher_Prefix{"hi"},
		})).To(Equal("hi"))
		Expect(utils.PathAsString(&matchers.Matcher{
			PathSpecifier: &matchers.Matcher_Regex{"howsitgoin"},
		})).To(Equal("howsitgoin"))
	})
	It("returns empty string for empty matcher", func() {
		Expect(utils.PathAsString(&matchers.Matcher{})).To(Equal(""))
		Expect(utils.PathAsString(nil)).To(Equal(""))
	})
})
