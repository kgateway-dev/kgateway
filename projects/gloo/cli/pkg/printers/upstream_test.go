package printers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("UpstreamTable", func() {
	It("handles malformed upstream (nil spec)", func() {
		Expect(func() {
			us := &v1.Upstream{}
			UpstreamTable(nil, []*v1.Upstream{us}, GinkgoWriter)
		}).NotTo(Panic())
	})
})
