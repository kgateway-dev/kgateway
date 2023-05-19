package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	sk_resources "github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("GlooResources", func() {
	us1 := createUpstream("us1", "ns1")
	us2 := createUpstream("us2", "ns2")
	us3 := createUpstream("us3", "ns3")
	us4 := createUpstream("us4", "ns4")
	us5 := createUpstream("us5", "ns5")

	Context("MergeResourceLists", func() {
		It("can merge disjoint lists", func() {
			existing := sk_resources.ResourceList{us3, us1}
			modified := sk_resources.ResourceList{us2, us5}
			merged := MergeResourceLists(existing, modified)
			Expect(merged).To(Equal(sk_resources.ResourceList{us1, us2, us3, us5}))
		})
		It("can merge overlapping lists", func() {
			existing := sk_resources.ResourceList{us1, us2, us3}
			modified := sk_resources.ResourceList{us4, us3, us2, us5}
			merged := MergeResourceLists(existing, modified)
			Expect(merged).To(Equal(sk_resources.ResourceList{us1, us2, us3, us4, us5}))
		})
		It("can merge a list into an empty list", func() {
			existing := sk_resources.ResourceList{}
			modified := sk_resources.ResourceList{us1, us4}
			merged := MergeResourceLists(existing, modified)
			Expect(merged).To(Equal(sk_resources.ResourceList{us1, us4}))
		})
		It("can merge an empty list into a list", func() {
			existing := sk_resources.ResourceList{us4, us3, us2}
			modified := sk_resources.ResourceList{}
			merged := MergeResourceLists(existing, modified)
			Expect(merged).To(Equal(sk_resources.ResourceList{us2, us3, us4}))
		})
		It("removes duplicates from a list", func() {
			existing := sk_resources.ResourceList{us1, us1, us5, us2, us5}
			modified := sk_resources.ResourceList{us1, us3}
			merged := MergeResourceLists(existing, modified)
			Expect(merged).To(Equal(sk_resources.ResourceList{us1, us2, us3, us5}))
		})
	})
	Context("DeleteResources", func() {
		It("can delete resources", func() {
			existing := sk_resources.ResourceList{us5, us1, us4, us2}
			refsToDelete := []*core.ResourceRef{
				{Name: "us1", Namespace: "ns1"},
				{Name: "us5", Namespace: "ns5"},
			}
			updatedList := DeleteResources(existing, refsToDelete)
			Expect(updatedList).To(Equal(sk_resources.ResourceList{us2, us4}))
		})
		It("can handle deletion of duplicates in resource list", func() {
			existing := sk_resources.ResourceList{us5, us1, us1, us4, us5, us2}
			refsToDelete := []*core.ResourceRef{
				{Name: "us1", Namespace: "ns1"},
			}
			updatedList := DeleteResources(existing, refsToDelete)
			Expect(updatedList).To(Equal(sk_resources.ResourceList{us2, us4, us5, us5}))
		})
		It("ignores refs that are not found", func() {
			existing := sk_resources.ResourceList{us1, us2, us3}
			refsToDelete := []*core.ResourceRef{
				{Name: "us1", Namespace: "ns1"},
				{Name: "us4", Namespace: "ns4"},
			}
			updatedList := DeleteResources(existing, refsToDelete)
			Expect(updatedList).To(Equal(sk_resources.ResourceList{us2, us3}))
		})
	})
})

func createUpstream(name string, namespace string) *v1.Upstream {
	return &v1.Upstream{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
	}
}
