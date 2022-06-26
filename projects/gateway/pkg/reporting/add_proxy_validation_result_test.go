package reporting_test

import (
	"context"

	"github.com/onsi/ginkgo/extensions/table"

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

	table.DescribeTable("Adds ProxyValidation errors to ResourceReports",
		func(translatorOptions translator.Opts) {
			snap = samples.SimpleGatewaySnapshot(&core.ResourceRef{Name: ignored, Namespace: ignored}, ignored)
			tx := translator.NewDefaultTranslator(translatorOptions)

			proxy, reports = tx.Translate(context.TODO(), ignored, snap, snap.Gateways)
			proxyReport := validation.MakeReport(proxy)

			for _, lis := range proxyReport.ListenerReports {
				validation.AppendListenerError(lis,
					validationapi.ListenerReport_Error_ProcessingError,
					"bad listener")

				availableVirtualHostReports := lis.GetHttpListenerReport().GetVirtualHostReports()
				for _, httpReport := range lis.GetAggregateListenerReport().GetHttpListenerReports() {
					availableVirtualHostReports = append(availableVirtualHostReports, httpReport.GetVirtualHostReports()...)
				}

				for _, vHost := range availableVirtualHostReports {
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
		},
		table.Entry("default translators", translator.Opts{
			WriteNamespace:                 ignored,
			IsolateVirtualHostsBySslConfig: false,
		}),
		table.Entry("isolated virtual hosts translators", translator.Opts{
			WriteNamespace:                 ignored,
			IsolateVirtualHostsBySslConfig: true,
		}),
	)
})
