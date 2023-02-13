package reporting_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/solo-io/gloo/projects/gateway/pkg/reporting"
)

var _ = Describe("AddProxyValidationResult", func() {

	var (
		snap    *gloov1snap.ApiSnapshot
		proxy   *gloov1.Proxy
		reports reporter.ResourceReports
		ignored = "ignored"
	)

	DescribeTable("Adds ProxyValidation errors to ResourceReports",
		func(translatorOptions translator.Opts) {
			snap = samples.SimpleGlooSnapshot(translatorOptions.WriteNamespace)
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
				for _, matchedReport := range lis.GetHybridListenerReport().GetMatchedListenerReports() {
					availableVirtualHostReports = append(availableVirtualHostReports, matchedReport.GetHttpListenerReport().GetVirtualHostReports()...)
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
				// These VirtualServices are now used by multiple listeners, and each listener adds 2 errors
				Expect(reports[vs].Errors.Error()).To(ContainSubstring(`4 errors occurred:
	* VirtualHost Error: DomainsNotUniqueError. Reason: bad vhost
	* Route Error: InvalidMatcherError. Reason: bad route. Route Name: route-0`))
			}
		},
		Entry("default translators", translator.Opts{
			WriteNamespace:                 ignored,
			IsolateVirtualHostsBySslConfig: false,
		}),
		Entry("isolated virtual hosts translators", translator.Opts{
			WriteNamespace:                 ignored,
			IsolateVirtualHostsBySslConfig: true,
		}),
	)
})
