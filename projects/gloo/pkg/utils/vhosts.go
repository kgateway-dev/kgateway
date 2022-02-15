package utils

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func GetVhostsFromListener(listener *v1.Listener) []*v1.VirtualHost {
	virtualHosts := []*v1.VirtualHost{}

	// extauth not yet supported for tcp
	switch typedListener := listener.GetListenerType().(type) {
	case *v1.Listener_HttpListener:
		virtualHosts = typedListener.HttpListener.GetVirtualHosts()
	case *v1.Listener_HybridListener:
		for _, matchedListener := range typedListener.HybridListener.GetMatchedListeners() {
			if httpListener := matchedListener.GetHttpListener(); httpListener != nil {
				virtualHosts = append(virtualHosts, httpListener.GetVirtualHosts()...)
			}
		}
	}
	return virtualHosts
}

func GetVhostReportsFromListenerReport(listenerReport *validation.ListenerReport) []*validation.VirtualHostReport {
	virtualHostReports := []*validation.VirtualHostReport{}

	switch listenerReportType := listenerReport.GetListenerTypeReport().(type) {
	case *validation.ListenerReport_HttpListenerReport:
		virtualHostReports = listenerReportType.HttpListenerReport.GetVirtualHostReports()
	case *validation.ListenerReport_HybridListenerReport:
		for _, matchedListenerReport := range listenerReportType.HybridListenerReport.GetMatchedListenerReports() {
			if httpListenerReport := matchedListenerReport.GetHttpListenerReport(); httpListenerReport != nil {
				virtualHostReports = append(virtualHostReports, httpListenerReport.GetVirtualHostReports()...)
			}
		}
	}
	return virtualHostReports
}
