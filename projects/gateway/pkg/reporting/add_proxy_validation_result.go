package reporting

import (
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	invalidReportsListenersErr    = errors.Errorf("internal err: reports did not match number of listeners")
	invalidReportsVirtualHostsErr = errors.Errorf("internal err: reports did not match number of virtual hosts")
	missingReportForSourceErr     = errors.Errorf("internal err: missing resource report for source resource")
)

// Update a set of ResourceReports with the results of a proxy validation
// Using the sources from Listener.Metadata, VirtualHost.Metadata, and Route.Metadata,
// we can extrapolate the errors
// this function should not return an error unless:
// - parsing the {listener/virtualhost/route}.Metadata struct as a translator.Sources fails
// - the proxy report does not match the proxy
func AddProxyValidationResult(resourceReports reporter.ResourceReports, proxy *gloov1.Proxy, proxyReport *validation.ProxyReport) error {
	listenerReports := proxyReport.GetListenerReports()
	if len(listenerReports) != len(proxy.GetListeners()) {
		return invalidReportsListenersErr
	}

	for i, listenerReport := range listenerReports {
		listener := proxy.GetListeners()[i]

		if err := addListenerResult(resourceReports, listener, listenerReport); err != nil {
			return err
		}

		if httpListenerReport := listenerReport.GetHttpListenerReport(); httpListenerReport != nil {
			vhReports := httpListenerReport.GetVirtualHostReports()
			virtualHosts := listener.GetHttpListener().GetVirtualHosts()

			if len(vhReports) != len(virtualHosts) {
				return invalidReportsVirtualHostsErr
			}

			for j, vhReport := range vhReports {
				virtualHost := virtualHosts[j]

				if err := addVirtualHostResult(resourceReports, virtualHost, vhReport); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func addListenerResult(resourceReports reporter.ResourceReports, listener *gloov1.Listener, listenerReport *validation.ListenerReport) error {
	listenerErrs := getListenerLevelErrors(listenerReport)

	return translator.ForEachSource(listener, func(src translator.SourceRef) error {
		srcResource, _ := resourceReports.Find(src.ResourceKind, &core.ResourceRef{Name: src.Name, Namespace: src.Namespace})
		if srcResource == nil {
			return missingReportForSourceErr
		}
		resourceReports.AddErrors(srcResource, listenerErrs...)
		return nil
	})
}

func addVirtualHostResult(resourceReports reporter.ResourceReports, virtualHost *gloov1.VirtualHost, vhReport *validation.VirtualHostReport) error {
	virtualHostErrs, virtualHostWarnings := getVirtualHostLevelErrorsAndWarnings(vhReport)

	return translator.ForEachSource(virtualHost, func(src translator.SourceRef) error {
		srcResource, _ := resourceReports.Find(src.ResourceKind, &core.ResourceRef{Name: src.Name, Namespace: src.Namespace})
		if srcResource == nil {
			return missingReportForSourceErr
		}
		resourceReports.AddErrors(srcResource, virtualHostErrs...)
		resourceReports.AddWarnings(srcResource, virtualHostWarnings...)
		return nil
	})
}

// get errors that can be caused by gateways
func getListenerLevelErrors(listenerReport *validation.ListenerReport) []error {
	listenerErrs := validationutils.GetListenerErr(listenerReport)

	switch listenerType := listenerReport.GetListenerTypeReport().(type) {
	case *validation.ListenerReport_HttpListenerReport:
		httpListener := listenerType.HttpListenerReport
		listenerErrs = append(listenerErrs, validationutils.GetHttpListenerErr(httpListener)...)

	case *validation.ListenerReport_TcpListenerReport:
		tcpListener := listenerType.TcpListenerReport
		listenerErrs = append(listenerErrs, validationutils.GetTcpListenerErr(tcpListener)...)

		for _, hostReport := range tcpListener.GetTcpHostReports() {
			listenerErrs = append(listenerErrs, validationutils.GetTcpHostErr(hostReport)...)
		}
	}

	return listenerErrs
}

// get errors that can be caused by virtual services
func getVirtualHostLevelErrorsAndWarnings(vhReport *validation.VirtualHostReport) ([]error, []string) {
	var (
		virtualHostErrs     = validationutils.GetVirtualHostErr(vhReport)
		virtualHostWarnings []string
	)

	for _, routeReport := range vhReport.GetRouteReports() {
		virtualHostErrs = append(virtualHostErrs, validationutils.GetRouteErr(routeReport)...)
		virtualHostWarnings = append(virtualHostWarnings, validationutils.GetRouteWarning(routeReport)...)
	}

	return virtualHostErrs, virtualHostWarnings
}
