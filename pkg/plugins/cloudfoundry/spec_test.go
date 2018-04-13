package cloudfoundry_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/plugins/cloudfoundry"
)

var _ = Describe("Spec", func() {
	var s UpstreamSpec
	BeforeEach(func() {
		s = UpstreamSpec{}
	})

	It("should succeed encoding", func() {
		s.Hostname = "solo.io"
		Expect(func() { EncodeUpstreamSpec(s) }).ToNot(Panic())
	})

	It("should be reversable", func() {
		s.Hostname = "solo.io"

		s2, err := DecodeUpstreamSpec(EncodeUpstreamSpec(s))
		Expect(err).NotTo(HaveOccurred())
		Expect(s2).To(Equal(&s))
	})

	It("should fail dencoding invalid spec", func() {
		_, err := DecodeUpstreamSpec(EncodeUpstreamSpec(s))
		Expect(err).To(HaveOccurred())
	})

})
