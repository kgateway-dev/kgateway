package gateway_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Url", func() {
	It("returns the correct url of a proxy pod", func() {

		// install gateway first
		err := testutils.Glooctl("install gateway --release 0.6.19")
		Expect(err).NotTo(HaveOccurred())

		addr, err := testutils.GlooctlOut("proxy url")
		Expect(err).NotTo(HaveOccurred())

		Expect(addr).To(HavePrefix("http://"))
	})
})
