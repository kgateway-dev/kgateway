package kubernetes_test

import (
	"strings"

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
	It("should truncate long names", func() {
		name := UpstreamName(strings.Repeat("y", 120), "gloo-system", 12, nil)
		Expect(name).To(HaveLen(63))
	})
	It("should truncate long names with lot of labels", func() {
		name := UpstreamName("test", "gloo-system", 12, map[string]string{"test": strings.Repeat("y", 120)})
		Expect(len(name)).To(BeNumerically("<=", 63))
	})

	It("should handle colisions", func() {
		name := UpstreamName(strings.Repeat("y", 120), "gloo-system", 12, nil)
		name2 := UpstreamName(strings.Repeat("y", 120)+"2", "gloo-system", 12, nil)
		Expect(name).ToNot(Equal(name2))
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
