package utils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/utils"
)

var _ = Describe("Namespaces", func() {

	var (
		allNamespaces = [][]string{nil, {}, {""}}
	)

	Context("all namespaces", func() {
		It("should consider empty list as all namespaces", func() {
			for _, testCase := range allNamespaces {
				Expect(utils.AllNamespaces(testCase)).To(BeTrue())
			}
		})

		It("should consider list with empty namespace as all namespaces", func() {
			Expect(utils.AllNamespaces([]string{""})).To(BeTrue())
		})

		It("should not consider non empty list as all namespaces", func() {
			Expect(utils.AllNamespaces([]string{"test"})).To(BeFalse())
		})
	})
	Context("process namespaces", func() {

		It("should not change all namespaces", func() {
			for _, testCase := range allNamespaces {
				Expect(utils.ProcessWatchNamespaces(testCase, "test")).To(Equal(testCase))
			}
		})
		It("should add write namespace if not there", func() {
			ns := []string{"ns1"}
			Expect(utils.ProcessWatchNamespaces(ns, "test")).To(Equal([]string{"ns1", "test"}))
		})

		It("should not add write namespace if already there", func() {
			ns := []string{"ns1"}
			Expect(utils.ProcessWatchNamespaces(ns, "ns1")).To(Equal([]string{"ns1"}))
		})

	})

})
