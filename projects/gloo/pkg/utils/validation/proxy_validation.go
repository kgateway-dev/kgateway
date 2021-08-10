package validation

import (
	"fmt"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"go.uber.org/multierr"
)

var (
	RouteErrorMsg      = "Route Error"
	RouteIdentifierTxt = "Route Name"
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

func mkErr(level, errType, reason string) error {
	return errors.Errorf("%v Error: %v. Reason: %v", level, errType, reason)
}

func GetListenerErr(listener *validation.ListenerReport) []error {
	var errs []error
	for _, errReport := range listener.GetErrors() {
		errs = append(errs, mkErr("Listener", errReport.GetType().String(), errReport.GetReason()))
	}
	return errs
}

func GetHttpListenerErr(httpListener *validation.HttpListenerReport) []error {
	var errs []error
	for _, errReport := range httpListener.GetErrors() {
		errs = append(errs, mkErr("HttpListener", errReport.GetType().String(), errReport.GetReason()))
	}
	return errs
}

func GetVirtualHostErr(virtualHost *validation.VirtualHostReport) []error {
	var errs []error
	for _, errReport := range virtualHost.GetErrors() {
		errs = append(errs, mkErr("VirtualHost", errReport.GetType().String(), errReport.GetReason()))
	}
	return errs
}

func GetRouteErr(route *validation.RouteReport) []error {
	var errs []error
	for _, errReport := range route.GetErrors() {
		routeError := errors.Errorf("%v. Reason: %v", errReport.GetType().String(), errReport.GetReason())
		errs = append(errs, errors.Wrap(routeError, RouteErrorMsg))
	}
	return errs
}

func GetRouteWarning(route *validation.RouteReport) []string {
	var warnings []string
	appendWarning := func(level, errType, reason string) {
		warnings = append(warnings, fmt.Sprintf("%v Warning: %v. Reason: %v", level, errType, reason))
	}

	for _, warning := range route.GetWarnings() {
		appendWarning("Route", warning.GetType().String(), warning.GetReason())
	}

	return warnings
}

func GetTcpListenerErr(tcpListener *validation.TcpListenerReport) []error {
	var errs []error
	for _, errReport := range tcpListener.GetErrors() {
		errs = append(errs, mkErr("TcpListener", errReport.GetType().String(), errReport.GetReason()))
	}
	return errs
}

func GetTcpHostErr(tcpHost *validation.TcpHostReport) []error {
	var errs []error
	for _, errReport := range tcpHost.GetErrors() {
		errs = append(errs, mkErr("TcpHost", errReport.GetType().String(), errReport.GetReason()))
	}
	return errs
}

func GetProxyError(proxyRpt *validation.ProxyReport) error {
	var errs []error
	for _, listener := range proxyRpt.GetListenerReports() {
		if err := GetListenerErr(listener); err != nil {
			errs = append(errs, err...)
		}
		switch listenerType := listener.GetListenerTypeReport().(type) {
		case *validation.ListenerReport_HttpListenerReport:
			httpListener := listenerType.HttpListenerReport
			if err := GetHttpListenerErr(httpListener); err != nil {
				errs = append(errs, err...)
			}
			for _, vhReport := range httpListener.GetVirtualHostReports() {
				if err := GetVirtualHostErr(vhReport); err != nil {
					errs = append(errs, err...)
				}
				for _, routeReport := range vhReport.GetRouteReports() {
					if err := GetRouteErr(routeReport); err != nil {
						errs = append(errs, err...)
					}
				}
			}
		case *validation.ListenerReport_TcpListenerReport:
			tcpListener := listenerType.TcpListenerReport
			if err := GetTcpListenerErr(tcpListener); err != nil {
				errs = append(errs, err...)
			}
			for _, hostReport := range tcpListener.GetTcpHostReports() {
				if err := GetTcpHostErr(hostReport); err != nil {
					errs = append(errs, err...)
				}
			}
		}
	}

	var combinedErr error
	for _, err := range errs {
		combinedErr = multierr.Append(combinedErr, err)
	}

	return combinedErr
}

func GetProxyWarning(proxyRpt *validation.ProxyReport) []string {
	var warnings []string

	for _, listener := range proxyRpt.GetListenerReports() {
		switch listenerType := listener.GetListenerTypeReport().(type) {
		case *validation.ListenerReport_HttpListenerReport:
			httpListener := listenerType.HttpListenerReport
			for _, vhReport := range httpListener.GetVirtualHostReports() {
				for _, routeReport := range vhReport.GetRouteReports() {
					if warns := GetRouteWarning(routeReport); len(warns) > 0 {
						warnings = append(warnings, warns...)
					}
				}
			}
		}
	}

	return warnings
}

func AppendListenerError(listenerReport *validation.ListenerReport, errType validation.ListenerReport_Error_Type, reason string) {
	listenerReport.Errors = append(listenerReport.GetErrors(), &validation.ListenerReport_Error{
		Type:   errType,
		Reason: reason,
	})
}

func AppendVirtualHostError(virtualHostReport *validation.VirtualHostReport, errType validation.VirtualHostReport_Error_Type, reason string) {
	virtualHostReport.Errors = append(virtualHostReport.GetErrors(), &validation.VirtualHostReport_Error{
		Type:   errType,
		Reason: reason,
	})
}

func AppendHTTPListenerError(httpListenerReport *validation.HttpListenerReport, errType validation.HttpListenerReport_Error_Type, reason string) {
	httpListenerReport.Errors = append(httpListenerReport.GetErrors(), &validation.HttpListenerReport_Error{
		Type:   errType,
		Reason: reason,
	})
}

func AppendRouteError(routeReport *validation.RouteReport, errType validation.RouteReport_Error_Type, reason string, routeName string) {
	routeReport.Errors = append(routeReport.GetErrors(), &validation.RouteReport_Error{
		Type:   errType,
		Reason: fmt.Sprintf("%s. %s: %s", reason, RouteIdentifierTxt, routeName),
	})
}

func AppendRouteWarning(routeReport *validation.RouteReport, errType validation.RouteReport_Warning_Type, reason string) {
	routeReport.Warnings = append(routeReport.GetWarnings(), &validation.RouteReport_Warning{
		Type:   errType,
		Reason: reason,
	})
}
