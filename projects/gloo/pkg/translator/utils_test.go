package translator_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {

	It("empty namespace: should convert upstream to cluster name and back properly", func() {
		ref := &core.ResourceRef{Name: "name", Namespace: ""}
		clusterName := translator.UpstreamToClusterName(ref)
		convertedBack, err := translator.ClusterToUpstreamRef(clusterName)
		Expect(err).ToNot(HaveOccurred())
		Expect(convertedBack).To(Equal(ref))
	})

	It("populated namespace: should convert upstream to cluster name and back properly", func() {
		ref := &core.ResourceRef{Name: "name", Namespace: "namespace"}
		clusterName := translator.UpstreamToClusterName(ref)
		convertedBack, err := translator.ClusterToUpstreamRef(clusterName)
		Expect(err).ToNot(HaveOccurred())
		Expect(convertedBack).To(Equal(ref))
	})

	DescribeTable(
		"GetIpv6Address",
		func(address, expectedAddress string, expectedErr error) {
			ipv6Address, err := translator.GetIpv6Address(address)

			if expectedErr != nil {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(ipv6Address).To(Equal(expectedAddress))
		},
		Entry("invalid ip returns original", "invalid", "invalid", errors.Errorf("bindAddress invalid is not a valid IP address")),
		Entry("ipv4 returns ipv4-mapped", "0.0.0.0", "::ffff:0.0.0.0", nil),
		Entry("ipv6 returns ipv6", "::", "::", nil),
	)

})
