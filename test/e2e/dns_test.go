package e2e_test

import (
	"regexp"
	"strconv"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"
	"google.golang.org/protobuf/types/known/durationpb"
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

		It("supports DnsRefreshRate", func() {
			Eventually(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				frequency := countRegexFrequency("dns_refresh_rate", cfg)
				g.Expect(frequency).To(Equal(0))
			}, "5s", ".5s").Should(Succeed(), "DnsRefreshRate not in ConfigDump")

			// Update the Upstream to include DnsRefreshRate in the definition
			testContext.PatchDefaultUpstream(func(us *gloov1.Upstream) *gloov1.Upstream {
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
// TODO (sam-heilbron) this does not seem to be working
func countRegexFrequency(matcher, text string) int {
	regex := regexp.MustCompile(matcher)
	matches := regex.FindAllStringSubmatch(text, -1)
	if len(matches) != 1 {
		return 0
	}

	// matches[0] is the first match
	// matches[0][1] is the first capture group
	matchCount, conversionErr := strconv.Atoi(matches[0][1])
	if conversionErr != nil {
		return 0
	}

	return matchCount
}
