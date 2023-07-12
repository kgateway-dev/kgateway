package utils

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func GetVirtualHostsForListener(listener *v1.Listener) []*v1.VirtualHost {
	var virtualHosts []*v1.VirtualHost

	switch typedListener := listener.GetListenerType().(type) {
	case *v1.Listener_HttpListener:
		virtualHosts = typedListener.HttpListener.GetVirtualHosts()
	case *v1.Listener_TcpListener:
		// VirtualHosts are an http-only concept
		break
	case *v1.Listener_HybridListener:
		for _, matchedListener := range typedListener.HybridListener.GetMatchedListeners() {
			if http, ok := matchedListener.GetListenerType().(*v1.MatchedListener_HttpListener); ok {
				virtualHosts = append(virtualHosts, http.HttpListener.GetVirtualHosts()...)
			}
		}
	case *v1.Listener_AggregateListener:
		for _, vhost := range typedListener.AggregateListener.GetHttpResources().GetVirtualHosts() {
			virtualHosts = append(virtualHosts, vhost)
		}
	default:
		break
	}

	return virtualHosts
}

func GetTcpHostReportsFromListenerReport(listenerReport *validation.ListenerReport) []*validation.TcpHostReport {
	var tcpHostReports []*validation.TcpHostReport

	switch listenerReportType := listenerReport.GetListenerTypeReport().(type) {
	case *validation.ListenerReport_HttpListenerReport:
		// HostReports are a tcp-only concept
		break
	case *validation.ListenerReport_TcpListenerReport:
		tcpHostReports = append(tcpHostReports, listenerReportType.TcpListenerReport.GetTcpHostReports()...)
	case *validation.ListenerReport_HybridListenerReport:
		for _, matchedListenerReport := range listenerReportType.HybridListenerReport.GetMatchedListenerReports() {
			if tcpListenerReport := matchedListenerReport.GetTcpListenerReport(); tcpListenerReport != nil {
				tcpHostReports = append(tcpHostReports, tcpListenerReport.GetTcpHostReports()...)
			}
		}
	case *validation.ListenerReport_AggregateListenerReport:
		for _, tcpListenerReport := range listenerReportType.AggregateListenerReport.GetTcpListenerReports() {
			tcpHostReports = append(tcpHostReports, tcpListenerReport.GetTcpHostReports()...)
		}
	default:
		break
	}

	return tcpHostReports
}

func GetVhostReportsFromListenerReport(listenerReport *validation.ListenerReport) []*validation.VirtualHostReport {
	var virtualHostReports []*validation.VirtualHostReport

	switch listenerReportType := listenerReport.GetListenerTypeReport().(type) {
	case *validation.ListenerReport_HttpListenerReport:
		virtualHostReports = listenerReportType.HttpListenerReport.GetVirtualHostReports()
	case *validation.ListenerReport_TcpListenerReport:
		// VirtualHosts are an http-only concept
		break
	case *validation.ListenerReport_HybridListenerReport:
		for _, matchedListenerReport := range listenerReportType.HybridListenerReport.GetMatchedListenerReports() {
			if httpListenerReport := matchedListenerReport.GetHttpListenerReport(); httpListenerReport != nil {
				virtualHostReports = append(virtualHostReports, httpListenerReport.GetVirtualHostReports()...)
			}
		}
	case *validation.ListenerReport_AggregateListenerReport:
		for _, httpListenerReport := range listenerReportType.AggregateListenerReport.GetHttpListenerReports() {
			virtualHostReports = append(virtualHostReports, httpListenerReport.GetVirtualHostReports()...)
		}
	default:
		break
	}
	return virtualHostReports
}
