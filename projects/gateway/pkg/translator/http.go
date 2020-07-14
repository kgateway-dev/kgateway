package translator

import (
	"context"
	"fmt"

	"github.com/solo-io/go-utils/hashutils"

	errors "github.com/rotisserie/eris"

	"k8s.io/apimachinery/pkg/labels"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var (
	NoVirtualHostErr = func(vs *v1.VirtualService) error {
		return errors.Errorf("virtual service [%s] does not specify a virtual host", vs.Metadata.Ref().Key())
	}
	DomainInOtherVirtualServicesErr = func(domain string, conflictingVsNames []string) error {
		if domain == "" {
			return errors.Errorf("domain conflict: other virtual services that belong to the same Gateway"+
				" as this one don't specify a domain (and thus default to '*'): %v", conflictingVsNames)
		}
		return errors.Errorf("domain conflict: the [%s] domain is present in other virtual services "+
			"that belong to the same Gateway as this one: %v", domain, conflictingVsNames)
	}
	GatewayHasConflictingVirtualServicesErr = func(conflictingDomains []string) error {
		var loggedDomains []string
		for _, domain := range conflictingDomains {
			if domain == "" {
				domain = "EMPTY_DOMAIN"
			}
			loggedDomains = append(loggedDomains, domain)
		}
		return errors.Errorf("domain conflict: the following domains are present in more than one of the "+
			"virtual services associated with this gateway: %v", loggedDomains)
	}
)

type HttpTranslator struct{}

func (t *HttpTranslator) GenerateListeners(ctx context.Context, snap *v1.ApiSnapshot, filteredGateways []*v1.Gateway, reports reporter.ResourceReports) []*gloov1.Listener {
	if len(snap.VirtualServices) == 0 {
		snapHash := hashutils.MustHash(snap)
		contextutils.LoggerFrom(ctx).Debugf("%v had no virtual services", snapHash)
		return nil
	}
	var result []*gloov1.Listener
	for _, gateway := range filteredGateways {
		if gateway.GetHttpGateway() == nil {
			continue
		}

		virtualServices := getVirtualServicesForGateway(gateway, snap.VirtualServices)
		validateVirtualServiceDomains(gateway, virtualServices, reports)
		listener := desiredListenerForHttp(gateway, virtualServices, snap.RouteTables, reports)
		result = append(result, listener)
	}
	return result
}

// Errors will be added to the report object.
func validateVirtualServiceDomains(gateway *v1.Gateway, virtualServices v1.VirtualServiceList, reports reporter.ResourceReports) {

	// Index the virtual services for this gateway by the domain
	vsByDomain := map[string]v1.VirtualServiceList{}
	for _, vs := range virtualServices {

		// Add warning and skip if no virtual host
		if vs.VirtualHost == nil {
			reports.AddWarning(vs, NoVirtualHostErr(vs).Error())
			continue
		}

		// Not specifying any domains is not an error per se, but we need to check whether multiple virtual services
		// don't specify any, so we use the empty string as a placeholder in this function.
		domains := append([]string{}, vs.VirtualHost.Domains...)
		if len(domains) == 0 {
			domains = []string{""}
		}

		for _, domain := range domains {
			vsByDomain[domain] = append(vsByDomain[domain], vs)
		}
	}

	var conflictingDomains []string
	for domain, vsWithThisDomain := range vsByDomain {
		if len(vsWithThisDomain) > 1 {
			conflictingDomains = append(conflictingDomains, domain)
			for i, vs := range vsWithThisDomain {
				var conflictingVsNames []string
				for j, otherVs := range vsWithThisDomain {
					if i != j {
						conflictingVsNames = append(conflictingVsNames, otherVs.Metadata.Ref().Key())
					}
				}
				reports.AddError(vs, DomainInOtherVirtualServicesErr(domain, conflictingVsNames))
			}
		}
	}
	if len(conflictingDomains) > 0 {
		reports.AddError(gateway, GatewayHasConflictingVirtualServicesErr(conflictingDomains))
	}
}

func getVirtualServicesForGateway(gateway *v1.Gateway, virtualServices v1.VirtualServiceList) v1.VirtualServiceList {

	var virtualServicesForGateway v1.VirtualServiceList
	for _, vs := range virtualServices {
		if GatewayContainsVirtualService(gateway, vs) {
			virtualServicesForGateway = append(virtualServicesForGateway, vs)
		}
	}

	return virtualServicesForGateway
}

func GatewayContainsVirtualService(gateway *v1.Gateway, virtualService *v1.VirtualService) bool {
	httpGateway := gateway.GetHttpGateway()
	if httpGateway == nil {
		return false
	}

	if gateway.Ssl != hasSsl(virtualService) {
		return false
	}

	if len(httpGateway.VirtualServiceSelector) > 0 {
		// select virtual services by the label selector
		selector := labels.SelectorFromSet(httpGateway.VirtualServiceSelector)

		vsLabels := labels.Set(virtualService.Metadata.Labels)

		return virtualServiceNamespaceValidForGateway(gateway, virtualService) && selector.Matches(vsLabels)
	}
	// use individual refs to collect virtual services
	virtualServiceRefs := httpGateway.VirtualServices

	if len(virtualServiceRefs) == 0 {
		return virtualServiceNamespaceValidForGateway(gateway, virtualService)
	}

	vsRef := virtualService.Metadata.Ref()

	for _, ref := range virtualServiceRefs {
		if ref == vsRef {
			return true
		}
	}

	return false
}

func virtualServiceNamespaceValidForGateway(gateway *v1.Gateway, virtualService *v1.VirtualService) bool {
	httpGateway := gateway.GetHttpGateway()
	if httpGateway == nil {
		return false
	}

	if len(httpGateway.VirtualServiceNamespaces) > 0 {
		for _, ns := range httpGateway.VirtualServiceNamespaces {
			if ns == "*" || virtualService.Metadata.Namespace == ns {
				return true
			}
		}
		return false
	}

	// by default, virtual services will be discovered in all namespaces
	return true
}

func hasSsl(vs *v1.VirtualService) bool {
	return vs.SslConfig != nil
}

func desiredListenerForHttp(gateway *v1.Gateway, virtualServicesForGateway v1.VirtualServiceList, tables v1.RouteTableList, reports reporter.ResourceReports) *gloov1.Listener {
	var (
		virtualHosts []*gloov1.VirtualHost
		sslConfigs   []*gloov1.SslConfig
	)

	for _, virtualService := range virtualServicesForGateway.Sort() {
		if virtualService.VirtualHost == nil {
			virtualService.VirtualHost = &v1.VirtualHost{}
		}
		vh, err := virtualServiceToVirtualHost(virtualService, tables, reports)
		if err != nil {
			reports.AddError(virtualService, err)
			continue
		}
		virtualHosts = append(virtualHosts, vh)
		if virtualService.SslConfig != nil {
			sslConfigs = append(sslConfigs, virtualService.SslConfig)
		}
	}

	var httpPlugins *gloov1.HttpListenerOptions
	if httpGateway := gateway.GetHttpGateway(); httpGateway != nil {
		httpPlugins = httpGateway.Options
	}
	listener := makeListener(gateway)
	listener.ListenerType = &gloov1.Listener_HttpListener{
		HttpListener: &gloov1.HttpListener{
			VirtualHosts: virtualHosts,
			Options:      httpPlugins,
		},
	}
	listener.SslConfigurations = sslConfigs

	if err := appendSource(listener, gateway); err != nil {
		// should never happen
		reports.AddError(gateway, err)
	}

	return listener
}

func virtualServiceToVirtualHost(vs *v1.VirtualService, tables v1.RouteTableList, reports reporter.ResourceReports) (*gloov1.VirtualHost, error) {
	converter := NewRouteConverter(NewRouteTableSelector(tables), NewRouteTableIndexer())
	routes, err := converter.ConvertVirtualService(vs, reports)
	if err != nil {
		// internal error, should never happen
		return nil, err
	}

	vh := &gloov1.VirtualHost{
		Name:    VirtualHostName(vs),
		Domains: vs.VirtualHost.Domains,
		Routes:  routes,
		Options: vs.VirtualHost.Options,
	}

	if err := appendSource(vh, vs); err != nil {
		// should never happen
		return nil, err
	}

	return vh, nil
}

func VirtualHostName(vs *v1.VirtualService) string {
	return fmt.Sprintf("%v.%v", vs.Metadata.Namespace, vs.Metadata.Name)
}
