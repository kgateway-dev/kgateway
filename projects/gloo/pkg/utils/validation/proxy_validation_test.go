package validation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
)

var _ = Describe("validation utils", func() {

	numListeners := 3
	numVhosts := 5
	numRoutes := 8
	makeProxy := func(http bool) *v1.Proxy {
		proxy := &v1.Proxy{}
		for i := 0; i < numListeners; i++ {
			if http {

				httpListener := &v1.HttpListener{}
				proxy.Listeners = append(proxy.Listeners, &v1.Listener{
					ListenerType: &v1.Listener_HttpListener{
						HttpListener: httpListener,
					},
				})

				for j := 0; j < numVhosts; j++ {
					vh := &v1.VirtualHost{}
					httpListener.VirtualHosts = append(httpListener.VirtualHosts, vh)

					for k := 0; k < numRoutes; k++ {
						vh.Routes = append(vh.Routes, &v1.Route{})
					}
				}
			} else {

				tcpListener := &v1.TcpListener{}
				proxy.Listeners = append(proxy.Listeners, &v1.Listener{
					ListenerType: &v1.Listener_TcpListener{
						TcpListener: tcpListener,
					},
				})

				for j := 0; j < numVhosts; j++ {
					vh := &v1.TcpHost{}
					tcpListener.TcpHosts = append(tcpListener.TcpHosts, vh)
				}
			}
		}
		return proxy
	}

	var _ = Describe("MakeReport", func() {
		It("generates a report which matches an http proxy", func() {

			proxy := makeProxy(true)

			rpt := MakeReport(proxy)
			Expect(rpt.ListenerReports).To(HaveLen(len(proxy.Listeners)))
			for i := range rpt.ListenerReports {
				vhReports := rpt.ListenerReports[i].GetHttpListenerReport().VirtualHostReports
				Expect(vhReports).To(HaveLen(len(proxy.Listeners[i].GetHttpListener().VirtualHosts)))
				for j := range vhReports {
					Expect(vhReports[i].RouteReports).To(HaveLen(len(proxy.Listeners[i].GetHttpListener().VirtualHosts[j].Routes)))

				}
			}
		})

		It("generates a report which matches a tcp proxy", func() {

			proxy := makeProxy(false)

			rpt := MakeReport(proxy)
			Expect(rpt.ListenerReports).To(HaveLen(len(proxy.Listeners)))
			for i := range rpt.ListenerReports {
				tcpHostReports := rpt.ListenerReports[i].GetTcpListenerReport().TcpHostReports
				Expect(tcpHostReports).To(HaveLen(len(proxy.Listeners[i].GetTcpListener().GetTcpHosts())))
			}

		})
	})

	var _ = Describe("GetProxyError", func() {
		It("aggregates the errors at every level", func() {
			rpt := MakeReport(makeProxy(true))

			rpt.ListenerReports[1].Errors = append(rpt.ListenerReports[1].Errors,
				&validation.ListenerReport_Error{
					Type:   validation.ListenerReport_Error_BindPortNotUniqueError,
					Reason: "bind port not unique",
				},
			)
			httpListenerReport := rpt.ListenerReports[2].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport
			httpListenerReport.Errors = append(httpListenerReport.Errors, &validation.HttpListenerReport_Error{
				Type:   validation.HttpListenerReport_Error_ProcessingError,
				Reason: "bad http plugin",
			})

			virtualHostReport := rpt.ListenerReports[0].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[2]

			virtualHostReport.Errors = append(virtualHostReport.Errors,
				&validation.VirtualHostReport_Error{
					Type:   validation.VirtualHostReport_Error_DomainsNotUniqueError,
					Reason: "domains not unique",
				},
			)
			routeReport := rpt.ListenerReports[1].ListenerTypeReport.(*validation.ListenerReport_HttpListenerReport).HttpListenerReport.VirtualHostReports[3].RouteReports[2]

			routeReport.Warnings = append(routeReport.Warnings,
				&validation.RouteReport_Warning{
					Type:   validation.RouteReport_Warning_InvalidDestinationWarning,
					Reason: "bad destination",
				},
			)

			err := GetProxyError(rpt)
			Expect(err).To(MatchError("VirtualHost Error: DomainsNotUniqueError. Reason: domains not unique; Listener Error: BindPortNotUniqueError. Reason: bind port not unique; HTTP Plugin Error: plugin. Reason: bad http plugin"))
		})
	})
})
