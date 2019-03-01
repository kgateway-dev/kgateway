package kubernetes_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
)

var _ = Describe("UdsConvert", func() {
	It("should get uniq label set", func() {

		svcSelector := map[string]string{"app": "foo"}
		podmetas := []map[string]string{
			map[string]string{"app": "foo", "env": "prod"},
			map[string]string{"app": "foo", "env": "prod"},
			map[string]string{"app": "foo", "env": "dev"},
		}
		result := GetUniqueLabelSetsForObjects(svcSelector, podmetas)
		expected := []map[string]string{
			map[string]string{"app": "foo"},
			map[string]string{"app": "foo", "env": "prod"},
			map[string]string{"app": "foo", "env": "dev"},
		}
		Expect(result).To(Equal(expected))
	})
	It("should ignore ignored labels", func() {

		svcSelector := map[string]string{"app": "foo"}
		podmetas := []map[string]string{
			map[string]string{"app": "foo", "env": "prod", "release": "first"},
		}
		result := GetUniqueLabelSetsForObjects(svcSelector, podmetas)
		expected := []map[string]string{
			map[string]string{"app": "foo"},
			map[string]string{"app": "foo", "env": "prod"},
		}
		Expect(result).To(Equal(expected))
	})
})
