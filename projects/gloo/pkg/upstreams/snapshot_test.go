package upstreams_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

var _ = Describe("hybrid upstream snapshot", func() {

	var snapshot upstreams.HybridUpstreamSnapshot

	BeforeEach(func() {
		snapshot = upstreams.NewHybridUpstreamSnapshot()
	})

	It("returns empty slice if snapshot is empty", func() {
		Expect(snapshot.ToList()).To(HaveLen(0))
	})

	It("correctly merges upstreams", func() {
		snapshot.SetUpstreams(v1.UpstreamList{kubeUs1, kubeUs2})
		usList := snapshot.ToList()
		Expect(usList).To(HaveLen(2))
		Expect(usList).To(ContainElement(kubeUs1))
		Expect(usList).To(ContainElement(kubeUs2))
	})

	It("correctly merges services", func() {
		snapshot.SetServices(skkube.ServiceList{svc1, svc2})
		usList := snapshot.ToList()
		Expect(usList).To(HaveLen(3))
	})

	Describe("hashing", func() {

		BeforeEach(func() {
			snapshot.SetUpstreams(v1.UpstreamList{kubeUs1})
			snapshot.SetServices(skkube.ServiceList{svc1})
			Expect(snapshot.ToList()).To(HaveLen(2))
		})

		It("hashes snapshot consistently", func() {
			hash1, hash2 := snapshot.Hash(), snapshot.Hash()
			Expect(hash1).To(Equal(hash2))
		})

		It("produces two different hashes if snapshot changed", func() {
			hash1 := snapshot.Hash()
			snapshot.SetUpstreams(v1.UpstreamList{kubeUs2})
			hash2 := snapshot.Hash()

			Expect(hash1).NotTo(Equal(hash2))
		})
	})

	It("clones correctly", func() {
		snapshot.SetUpstreams(v1.UpstreamList{kubeUs1})
		before := snapshot.Clone().ToList()
		Expect(before).To(ConsistOf(kubeUs1))

		snapshot.SetUpstreams(v1.UpstreamList{kubeUs2})
		after := snapshot.ToList()
		Expect(after).To(ConsistOf(kubeUs2))
	})
})

var kubeUs1 = getUpstream("us-kube-1", "ns-1", "svc-1", "svn-ns-1", 1234)
var kubeUs2 = getUpstream("us-kube-2", "ns-2", "svc-2", "svn-ns-2", 4312)

var svc1 = getService("svc-1", "ns-1", "1", []int32{80})
var svc2 = getService("svc-2", "ns-1", "1", []int32{8080, 8081})
