package reporting_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gateway/pkg/reporting"
)

var _ = Describe("CheckSourceReports", func() {

	var (
		snap    *v1.ApiSnapshot
		proxy   *gloov1.Proxy
		reports reporter.ResourceReports
		ignored = "ignored"
	)
	BeforeEach(func() {
		snap = samples.SimpleGatewaySnapshot(&core.ResourceRef{Name: ignored, Namespace: ignored}, ignored)
		tx := translator.NewDefaultTranslator(translator.Opts{})
		proxy, reports = tx.Translate(context.TODO(), ignored, ignored, snap, snap.Gateways)
	})
	It("returns true when all the sources for the config object are error-free", func() {
		accepted, err := AllSourcesAccepted(reports, proxy.Listeners[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(accepted).To(BeTrue())
	})
	It("returns true when false when any sources for the config object have an error", func() {
		gwReport := reports[snap.Gateways[0]]
		gwReport.Errors = errors.Errorf("i did an oopsie")
		reports[snap.Gateways[0]] = gwReport

		accepted, err := AllSourcesAccepted(reports, proxy.Listeners[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(accepted).To(BeFalse())

		// listener 2 has a different source
		accepted, err = AllSourcesAccepted(reports, proxy.Listeners[1])
		Expect(err).NotTo(HaveOccurred())
		Expect(accepted).To(BeTrue())
	})
})
