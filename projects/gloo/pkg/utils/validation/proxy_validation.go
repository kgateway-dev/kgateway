package validation

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"go.uber.org/multierr"
)

func MakeReport(proxy *v1.Proxy) *validation.ProxyReport {
	listeners := proxy.GetListeners()
	listenerReports := make([]*validation.ListenerReport, len(listeners))

	for i, listener := range listeners {
		switch listenerType := listener.GetListenerType().(type) {
		case *v1.Listener_HttpListener:

			vhostReports := make([]*validation.VirtualHostReport, len(listenerType.HttpListener.GetVirtualHosts()))

			for j, vh := range listenerType.HttpListener.GetVirtualHosts() {
				routeReports := make([]*validation.RouteReport, len(vh.GetRoutes()))
				for k := range vh.GetRoutes() {
					routeReports[k] = &validation.RouteReport{}
				}

				vhostReports[j] = &validation.VirtualHostReport{
					RouteReports: routeReports,
				}
			}

			listenerReports[i] = &validation.ListenerReport{
				ListenerTypeReport: &validation.ListenerReport_HttpListenerReport{
					HttpListenerReport: &validation.HttpListenerReport{
						VirtualHostReports: vhostReports,
					},
				},
			}
		case *v1.Listener_TcpListener:
			tcpHostReports := make([]*validation.TcpHostReport, len(listenerType.TcpListener.GetTcpHosts()))
			for j := range listenerType.TcpListener.GetTcpHosts() {
				tcpHostReports[j] = &validation.TcpHostReport{}
			}
			listenerReports[i] = &validation.ListenerReport{
				ListenerTypeReport: &validation.ListenerReport_TcpListenerReport{
					TcpListenerReport: &validation.TcpListenerReport{
						TcpHostReports: tcpHostReports,
					},
				},
			}
		}
	}

	return &validation.ProxyReport{
		ListenerReports: listenerReports,
	}
}

func GetProxyError(proxyRpt *validation.ProxyReport) error {
	var errs error
	appendErr := func(level, errType, reason string) {
		errs = multierr.Append(errs, errors.Errorf("%v Error: %v. Reason: %v", level, errType, reason))

	}
	for _, listener := range proxyRpt.GetListenerReports() {
		for _, errReport := range listener.GetErrors() {
			appendErr("Listener", errReport.Type.String(), errReport.Reason)
		}
		switch listenerType := listener.ListenerTypeReport.(type) {
		case *validation.ListenerReport_HttpListenerReport:
			httpListener := listenerType.HttpListenerReport
			for _, errReport := range httpListener.GetErrors() {
				appendErr("HTTP Plugin", "plugin", errReport.Reason)
			}
			for _, vhReport := range httpListener.GetVirtualHostReports() {
				for _, errReport := range vhReport.GetErrors() {
					appendErr("VirtualHost", errReport.Type.String(), errReport.Reason)
				}
				for _, routeReport := range vhReport.GetRouteReports() {
					for _, errReport := range routeReport.GetErrors() {
						appendErr("Route", errReport.Type.String(), errReport.Reason)
					}
				}
			}
		case *validation.ListenerReport_TcpListenerReport:
			tcpListener := listenerType.TcpListenerReport
			for _, errReport := range tcpListener.GetErrors() {
				appendErr("TCP Listener", errReport.Type.String(), errReport.Reason)
			}

			for _, hostReport := range tcpListener.GetTcpHostReports() {
				for _, errReport := range hostReport.GetErrors() {
					appendErr("TCP Host", errReport.Type.String(), errReport.Reason)
				}
			}
		}
	}

	return errs
}

func GetProxyWarning(proxyRpt *validation.ProxyReport) []string {
	var warnings []string
	appendWarning := func(level, errType, reason string) {
		warnings = append(warnings, fmt.Sprintf("%v Warning: %v. Reason: %v", level, errType, reason))
	}
	for _, listener := range proxyRpt.GetListenerReports() {
		switch listenerType := listener.ListenerTypeReport.(type) {
		case *validation.ListenerReport_HttpListenerReport:
			httpListener := listenerType.HttpListenerReport
			for _, vhReport := range httpListener.GetVirtualHostReports() {
				for _, routeReport := range vhReport.GetRouteReports() {
					for _, warning := range routeReport.GetWarnings() {
						appendWarning("Route", warning.Type.String(), warning.Reason)
					}
				}
			}
		}
	}

	return warnings
}

func AppendListenerError(listenerReport *validation.ListenerReport, errType validation.ListenerReport_Error_Type, reason string) {
	listenerReport.Errors = append(listenerReport.Errors, &validation.ListenerReport_Error{
		Type:   errType,
		Reason: reason,
	})
}

func AppendVirtualHostError(virtualHostReport *validation.VirtualHostReport, errType validation.VirtualHostReport_Error_Type, reason string) {
	virtualHostReport.Errors = append(virtualHostReport.Errors, &validation.VirtualHostReport_Error{
		Type:   errType,
		Reason: reason,
	})
}

func AppendHTTPListenerError(httpListenerReport *validation.HttpListenerReport, errType validation.HttpListenerReport_Error_Type, reason string) {
	httpListenerReport.Errors = append(httpListenerReport.Errors, &validation.HttpListenerReport_Error{
		Type:   errType,
		Reason: reason,
	})
}

func AppendRouteError(routeReport *validation.RouteReport, errType validation.RouteReport_Error_Type, reason string) {
	routeReport.Errors = append(routeReport.Errors, &validation.RouteReport_Error{
		Type:   errType,
		Reason: reason,
	})
}

func AppendRouteWarning(routeReport *validation.RouteReport, errType validation.RouteReport_Warning_Type, reason string) {
	routeReport.Warnings = append(routeReport.Warnings, &validation.RouteReport_Warning{
		Type:   errType,
		Reason: reason,
	})
}
