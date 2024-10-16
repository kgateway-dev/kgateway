package namespaces_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/utils/namespaces"
)

var _ = Describe("Namespaces", func() {

	var (
		allNamespaces = [][]string{nil, {}, {""}}
	)

	Context("all namespaces", func() {
		It("should consider empty list as all namespaces", func() {
			for _, testCase := range allNamespaces {
				Expect(namespaces.AllNamespaces(testCase)).To(BeTrue())
			}
		})

		It("should consider list with empty namespace as all namespaces", func() {
			Expect(namespaces.AllNamespaces([]string{""})).To(BeTrue())
		})

		It("should not consider non empty list as all namespaces", func() {
			Expect(namespaces.AllNamespaces([]string{"test"})).To(BeFalse())
		})
	})
	Context("process namespaces", func() {

		It("should not change all namespaces", func() {
			for _, testCase := range allNamespaces {
				Expect(namespaces.ProcessWatchNamespaces(testCase, "test")).To(Equal(testCase))
			}
		})
		It("should add write namespace if not there", func() {
			ns := []string{"ns1"}
			Expect(namespaces.ProcessWatchNamespaces(ns, "test")).To(Equal([]string{"ns1", "test"}))
		})

		It("should not add write namespace if already there", func() {
			ns := []string{"ns1"}
			Expect(namespaces.ProcessWatchNamespaces(ns, "ns1")).To(Equal([]string{"ns1"}))
		})

	})

})
