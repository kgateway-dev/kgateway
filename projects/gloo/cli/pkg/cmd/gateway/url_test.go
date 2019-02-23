package gateway_test

import (
	. "github.com/onsi/ginkgo"
)

var _ = Describe("Url", func() {
	It("returns the correct url of a proxy pod", func() {

		// TODO(marco): temporarily disable this test, it relies on an old version of the Helm chart

		// install gateway first
		//err := testutils.Glooctl("install gateway --release 0.6.19")
		//Expect(err).NotTo(HaveOccurred())
		//
		//addr, err := testutils.GlooctlOut("proxy url")
		//Expect(err).NotTo(HaveOccurred())
		//
		//Expect(addr).To(HavePrefix("http://"))
	})
})
