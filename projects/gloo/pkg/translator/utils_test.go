package translator_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Utils", func() {

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

	DescribeTable(
		"IsIpv4Address",
		func(address string, expectedIpv4 bool, expectedErr error) {
			_, isIpv4Address, err := translator.IsIpv4Address(address)

			if expectedErr != nil {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(isIpv4Address).To(Equal(expectedIpv4))
		},
		Entry("invalid ip returns original", "invalid", false, errors.Errorf("bindAddress invalid is not a valid IP address")),
		Entry("ipv4 returns true", "0.0.0.0", true, nil),
		Entry("ipv6 returns false", "::", false, nil),
		Entry("ipv4inipv6", "::ffff:0.0.0.0", false, nil),
	)

})
