package validation

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

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

			listenerReports[i] = &validation.ListenerReport{
				ListenerTypeReport: &validation.ListenerReport_HttpListenerReport{
					HttpListenerReport: &validation.HttpListenerReport{
						VirtualHostReports: makeVhostReports(listenerType.HttpListener.GetVirtualHosts()),
					},
				},
			}
		case *v1.Listener_TcpListener:
			listenerReports[i] = &validation.ListenerReport{
				ListenerTypeReport: &validation.ListenerReport_TcpListenerReport{
					TcpListenerReport: &validation.TcpListenerReport{
						TcpHostReports: makeTcpHostReports(listenerType.TcpListener.GetTcpHosts()),
					},
				},
			}
		case *v1.Listener_HybridListener:
			matchedListenerReports := make(map[string]*validation.MatchedListenerReport)
			for _, matchedListener := range listenerType.HybridListener.GetMatchedListeners() {
				matchedListenerReport := &validation.MatchedListenerReport{}
				switch matchedListenerType := matchedListener.GetListenerType().(type) {
				case *v1.MatchedListener_HttpListener:
					matchedListenerReport.ListenerReportType = &validation.MatchedListenerReport_HttpListenerReport{
						HttpListenerReport: &validation.HttpListenerReport{
							VirtualHostReports: makeVhostReports(matchedListenerType.HttpListener.GetVirtualHosts()),
						},
					}
				case *v1.MatchedListener_TcpListener:
					matchedListenerReport.ListenerReportType = &validation.MatchedListenerReport_TcpListenerReport{
						TcpListenerReport: &validation.TcpListenerReport{
							TcpHostReports: makeTcpHostReports(matchedListenerType.TcpListener.GetTcpHosts()),
						},
					}
				}
				matchedListenerReports[utils.MatchedRouteConfigName(listener, matchedListener.GetMatcher())] = matchedListenerReport
			}

			listenerReports[i] = &validation.ListenerReport{
				ListenerTypeReport: &validation.ListenerReport_HybridListenerReport{
					HybridListenerReport: &validation.HybridListenerReport{
						MatchedListenerReports: matchedListenerReports,
					},
				},
			}
		case *v1.Listener_AggregateListener:
			httpListenerReports := make(map[string]*validation.HttpListenerReport)
			tcpListenerReports := make(map[string]*validation.TcpListenerReport)
			httpResources := listenerType.AggregateListener.GetHttpResources()
			for _, httpFilterChain := range listenerType.AggregateListener.GetHttpFilterChains() {
				var virtualHosts []*v1.VirtualHost
				for _, vhostRef := range httpFilterChain.GetVirtualHostRefs() {
					virtualHosts = append(virtualHosts, httpResources.GetVirtualHosts()[vhostRef])
				}

				httpListenerReport := &validation.HttpListenerReport{
					VirtualHostReports: makeVhostReports(virtualHosts),
				}
				httpListenerReports[utils.MatchedRouteConfigName(listener, httpFilterChain.GetMatcher())] = httpListenerReport
			}

			for _, tcpListener := range listenerType.AggregateListener.GetTcpListeners() {
				tcpListenerReport := &validation.TcpListenerReport{
					TcpHostReports: makeTcpHostReports(tcpListener.GetTcpListener().GetTcpHosts()),
				}
				tcpListenerReports[utils.MatchedRouteConfigName(listener, tcpListener.GetMatcher())] = tcpListenerReport
			}

			listenerReports[i] = &validation.ListenerReport{
				ListenerTypeReport: &validation.ListenerReport_AggregateListenerReport{
					AggregateListenerReport: &validation.AggregateListenerReport{
						HttpListenerReports: httpListenerReports,
						TcpListenerReports:  tcpListenerReports,
					},
				},
			}

		}
	}

	return &validation.ProxyReport{
		ListenerReports: listenerReports,
	}
}

func makeVhostReports(virtualHosts []*v1.VirtualHost) []*validation.VirtualHostReport {
	vhostReports := make([]*validation.VirtualHostReport, len(virtualHosts))

	for j, vh := range virtualHosts {
		routeReports := make([]*validation.RouteReport, len(vh.GetRoutes()))
		for k := range vh.GetRoutes() {
			routeReports[k] = &validation.RouteReport{}
		}

		vhostReports[j] = &validation.VirtualHostReport{
			RouteReports: routeReports,
		}
	}

	return vhostReports
}

func makeTcpHostReports(tcpHosts []*v1.TcpHost) []*validation.TcpHostReport {
	tcpHostReports := make([]*validation.TcpHostReport, len(tcpHosts))
	for j := range tcpHosts {
		tcpHostReports[j] = &validation.TcpHostReport{}
	}
	return tcpHostReports
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
			errs = append(errs, getHttpListenerReportErrs(listenerType.HttpListenerReport)...)

		case *validation.ListenerReport_TcpListenerReport:
			errs = append(errs, getTcpListenerReportErrs(listenerType.TcpListenerReport)...)

		case *validation.ListenerReport_HybridListenerReport:
			for _, mr := range listenerType.HybridListenerReport.GetMatchedListenerReports() {
				switch lrt := mr.GetListenerReportType().(type) {
				case *validation.MatchedListenerReport_HttpListenerReport:
					errs = append(errs, getHttpListenerReportErrs(lrt.HttpListenerReport)...)
				case *validation.MatchedListenerReport_TcpListenerReport:
					errs = append(errs, getTcpListenerReportErrs(lrt.TcpListenerReport)...)
				}
			}

		case *validation.ListenerReport_AggregateListenerReport:
			for _, httpListenerReport := range listenerType.AggregateListenerReport.GetHttpListenerReports() {
				errs = append(errs, getHttpListenerReportErrs(httpListenerReport)...)
			}
			for _, tcpListenerReport := range listenerType.AggregateListenerReport.GetTcpListenerReports() {
				errs = append(errs, getTcpListenerReportErrs(tcpListenerReport)...)
			}
		}
	}

	var combinedErr error
	for _, err := range errs {
		combinedErr = multierr.Append(combinedErr, err)
	}

	return combinedErr
}

func getTcpListenerReportErrs(tcpListenerReport *validation.TcpListenerReport) []error {
	var errs []error

	if err := GetTcpListenerErr(tcpListenerReport); err != nil {
		errs = append(errs, err...)
	}
	for _, hostReport := range tcpListenerReport.GetTcpHostReports() {
		if err := GetTcpHostErr(hostReport); err != nil {
			errs = append(errs, err...)
		}
	}

	return errs
}

func getHttpListenerReportErrs(httpListenerReport *validation.HttpListenerReport) []error {
	var errs []error

	if err := GetHttpListenerErr(httpListenerReport); err != nil {
		errs = append(errs, err...)
	}
	for _, vhReport := range httpListenerReport.GetVirtualHostReports() {
		if err := GetVirtualHostErr(vhReport); err != nil {
			errs = append(errs, err...)
		}
		for _, routeReport := range vhReport.GetRouteReports() {
			if err := GetRouteErr(routeReport); err != nil {
				errs = append(errs, err...)
			}
		}
	}

	return errs
}

func GetProxyWarning(proxyRpt *validation.ProxyReport) []string {
	var warnings []string

	for _, listenerReport := range proxyRpt.GetListenerReports() {
		vhostReports := utils.GetVhostReportsFromListenerReport(listenerReport)
		for _, vhReport := range vhostReports {
			for _, routeReport := range vhReport.GetRouteReports() {
				if warns := GetRouteWarning(routeReport); len(warns) > 0 {
					warnings = append(warnings, warns...)
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

func AppendTCPListenerError(httpListenerReport *validation.TcpListenerReport, errType validation.TcpListenerReport_Error_Type, reason string) {
	httpListenerReport.Errors = append(httpListenerReport.GetErrors(), &validation.TcpListenerReport_Error{
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
