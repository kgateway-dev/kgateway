package printers

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("UpstreamTable", func() {
	It("handles malformed upstream (nil spec)", func() {
		Expect(func() {
			us := &v1.Upstream{}
			UpstreamTable([]*v1.Upstream{us}, cliutil.GetLogger())
		}).NotTo(Panic())
	})
})
