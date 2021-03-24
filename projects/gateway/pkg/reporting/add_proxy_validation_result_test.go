package reporting_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gateway/pkg/reporting"
)

var _ = Describe("AddProxyValidationResult", func() {
	var (
		snap    *v1.ApiSnapshot
		proxy   *gloov1.Proxy
		reports reporter.ResourceReports
		ignored = "ignored"
	)
	BeforeEach(func() {
		snap = samples.SimpleGatewaySnapshot(&core.ResourceRef{Name: ignored, Namespace: ignored}, ignored)
		tx := translator.NewTranslator([]translator.ListenerFactory{&translator.HttpTranslator{}, &translator.TcpTranslator{}}, translator.Opts{})
		proxy, reports = tx.Translate(context.TODO(), ignored, ignored, snap, snap.Gateways)
	})
	It("it adds proxy validation errors to the resource reports", func() {
		proxyReport := validation.MakeReport(proxy)

		for _, lis := range proxyReport.ListenerReports {
			validation.AppendListenerError(lis,
				validationapi.ListenerReport_Error_ProcessingError,
				"bad listener")

			for _, vHost := range lis.GetHttpListenerReport().GetVirtualHostReports() {
				validation.AppendVirtualHostError(vHost,
					validationapi.VirtualHostReport_Error_DomainsNotUniqueError,
					"bad vhost")

				for _, route := range vHost.GetRouteReports() {
					validation.AppendRouteError(route,
						validationapi.RouteReport_Error_InvalidMatcherError,
						"bad route",
						"route-0",
					)
				}
			}
		}

		err := AddProxyValidationResult(reports, proxy, proxyReport)
		Expect(err).NotTo(HaveOccurred())

		for _, gw := range snap.Gateways {
			Expect(reports[gw].Errors).To(HaveOccurred())
			Expect(reports[gw].Errors.Error()).To(ContainSubstring(`1 error occurred:
	* Listener Error: ProcessingError. Reason: bad listener`))
		}

		for _, vs := range snap.VirtualServices {
			Expect(reports[vs].Errors).To(HaveOccurred())
			Expect(reports[vs].Errors.Error()).To(ContainSubstring(`2 errors occurred:
	* VirtualHost Error: DomainsNotUniqueError. Reason: bad vhost
	* Route Error: InvalidMatcherError. Reason: bad route. Route Name: route-0`))
		}
	})
})
