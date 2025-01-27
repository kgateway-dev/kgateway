package e2e_test

import (
	"regexp"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"
)

var _ = Describe("DNS E2E Test", func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("Defined on an Upstream", func() {
		// It would be preferable to assert behaviors
		// However, in the short term, we assert that the configuration has been received by the gateway-proxy

		It("ignores DnsRefreshRate on STATIC cluster", func() {
			Eventually(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				frequency := countRegexFrequency("dns_refresh_rate", cfg)
				g.Expect(frequency).To(Equal(0))
			}, "5s", ".5s").Should(Succeed(), "DnsRefreshRate not in ConfigDump")

			// Update the Upstream to include DnsRefreshRate in the definition
			// This is a STATIC Upstream, so we would expect the resource to have a warning on it
			// and not propagate the configuration
			testContext.PatchDefaultUpstream(func(us *gloov1.Upstream) *gloov1.Upstream {
				us.DnsRefreshRate = &durationpb.Duration{Seconds: 10}
				return us
			})

			Consistently(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				frequency := countRegexFrequency("dns_refresh_rate", cfg)
				g.Expect(frequency).To(Equal(0))
			}, "2s", ".5s").Should(Succeed(), "DnsRefreshRate still not in ConfigDump")
		})

		It("supports DnsRefreshRate on STRICT_DNS cluster", func() {
			Eventually(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				frequency := countRegexFrequency("dns_refresh_rate", cfg)
				g.Expect(frequency).To(Equal(0))
			}, "5s", ".5s").Should(Succeed(), "DnsRefreshRate not in ConfigDump")

			// Update the Upstream to have a non-IP host
			// This will cause the generated cluster to be STRICT_DNS and the control plane
			// will accept the DnsRefreshRate change
			testContext.PatchDefaultUpstream(func(us *gloov1.Upstream) *gloov1.Upstream {
				us.GetStatic().Hosts[0].Addr = "non-ip-host"
				us.DnsRefreshRate = &durationpb.Duration{Seconds: 10}
				return us
			})

			Eventually(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				frequency := countRegexFrequency("dns_refresh_rate", cfg)
				g.Expect(frequency).To(Equal(1))
			}, "5s", ".5s").Should(Succeed(), "DnsRefreshRate in ConfigDump")
		})

		It("supports RespectDnsTtl", func() {
			// Some bootstrap clusters have respect_dns_ttl enabled, so we first count the frequency
			originalFrequency := 0

			Eventually(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				originalFrequency = countRegexFrequency("respect_dns_ttl", cfg)
				g.Expect(originalFrequency).NotTo(Equal(0))
			}, "5s", ".5s").Should(Succeed(), "Count initial RespectDnsTtl in ConfigDump")

			// Update the Upstream to include RespectDnsTtl in the definition
			testContext.PatchDefaultUpstream(func(us *gloov1.Upstream) *gloov1.Upstream {
				us.RespectDnsTtl = &wrappers.BoolValue{Value: true}
				return us
			})

			Eventually(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				newFrequency := countRegexFrequency("respect_dns_ttl", cfg)
				g.Expect(newFrequency).To(Equal(originalFrequency + 1))
			}, "5s", ".5s").Should(Succeed(), "RespectDnsTtl count increased by 1")
		})
	})

})

// countRegexFrequency returns the frequency of a `matcher` within a `text`
func countRegexFrequency(matcher, text string) int {
	regex := regexp.MustCompile(matcher)
	matches := regex.FindAllStringSubmatch(text, -1)

	return len(matches)
}
