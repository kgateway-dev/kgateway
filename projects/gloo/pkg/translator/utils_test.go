package translator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	upstreams_kube "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
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

	Context("UpstreamToClusterStatsName", func() {

		DescribeTable("converting upstream to cluster stats name",
			func(us *gloov1.Upstream, expectedStatsName string) {
				statsName := translator.UpstreamToClusterStatsName(us)
				Expect(statsName).To(Equal(expectedStatsName))
			},
			Entry("real kube upstream", createKubeUpstream(true, "name", "ns", "svcName", "svcNs", 123), "kube-upstream_name_ns_svcNs_svcName_123"),
			Entry("fake kube upstream", createKubeUpstream(false, "name", "ns", "svcName", "svcNs", 123), "kube-svc_name_ns_svcNs_svcName_123"),
			Entry("non-kube upstream", createStaticUpstream("name", "ns"), "name_ns"),
		)
	})

	DescribeTable(
		"IsIpv4Address",
		func(address string, expectedIPv4, expectedPureIPv4 bool, expectedErr error) {
			isIPv4Address, isPureIPv4Address, err := translator.IsIpv4Address(address)

			if expectedErr != nil {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(isIPv4Address).To(Equal(expectedIPv4))
			Expect(isPureIPv4Address).To(Equal(expectedPureIPv4))
		},
		Entry("invalid ip returns original", "invalid", false, false, errors.Errorf("bindAddress invalid is not a valid IP address")),
		Entry("ipv4 returns true", "0.0.0.0", true, true, nil),
		Entry("ipv6 returns false", "::", false, false, nil),
		Entry("ipv4 mapped in ipv6", "::ffff:0.0.0.0", true, false, nil),
	)

})

func createKubeUpstream(isReal bool, name string, namespace string,
	svcName string, svcNamespace string, svcPort uint32) *gloov1.Upstream {
	if !isReal {
		name = upstreams_kube.FakeUpstreamNamePrefix + name
	}
	return &gloov1.Upstream{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		UpstreamType: &gloov1.Upstream_Kube{
			Kube: &kubernetes.UpstreamSpec{
				ServiceName:      svcName,
				ServiceNamespace: svcNamespace,
				ServicePort:      svcPort,
			},
		},
	}
}
