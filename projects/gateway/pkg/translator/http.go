package translator

import (
	"context"
	"fmt"
	"regexp"

	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"

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
	DomainInOtherVirtualServicesErr = func(domain string, conflictingVsRefs []string) error {
		if domain == "" {
			return errors.Errorf("domain conflict: other virtual services that belong to the same Gateway"+
				" as this one don't specify a domain (and thus default to '*'): %v", conflictingVsRefs)
		}
		return errors.Errorf("domain conflict: the [%s] domain is present in other virtual services "+
			"that belong to the same Gateway as this one: %v", domain, conflictingVsRefs)
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
	ConflictingMatcherErr = func(vh string, matcher *matchers.Matcher) error {
		return errors.Errorf("virtual host [%s] has conflicting matcher: %v", vh, matcher)
	}
	UnorderedRoutesErr = func(vh, rt1Name, rt2Name string, rt1Matcher, rt2Matcher *matchers.Matcher) error {
		return errors.Errorf("virtual host [%s] has unordered routes; expected route named [%s] with matcher "+
			"[%v] to come before route named [%s] with matcher [%v]", vh, rt1Name, rt1Matcher, rt2Name, rt2Matcher)
	}
	UnorderedRegexErr = func(vh, regex string, matcher *matchers.Matcher) error {
		return errors.Errorf("virtual host [%s] has unordered regex routes, earlier regex [%s] matched later "+
			"route [%v]", vh, regex, matcher)
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
		if ref.Equal(vsRef) {
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

	validateRoutes(vs, vh, reports)

	if err := appendSource(vh, vs); err != nil {
		// should never happen
		return nil, err
	}

	return vh, nil
}

func VirtualHostName(vs *v1.VirtualService) string {
	return fmt.Sprintf("%v.%v", vs.Metadata.Namespace, vs.Metadata.Name)
}

// this function is written with the assumption that the routes will not be modified afterwards,
// and are in their final sorted form
func validateRoutes(vs *v1.VirtualService, vh *gloov1.VirtualHost, reports reporter.ResourceReports) {
	validateAnyDuplicateMatchers(vs, vh, reports)
	validateRouteOrder(vs, vh, reports)
	validateRegexHijacking(vs, vh, reports)
}

func validateAnyDuplicateMatchers(vs *v1.VirtualService, vh *gloov1.VirtualHost, reports reporter.ResourceReports) {
	// warn on duplicate matchers
	seenMatchers := make(map[uint64]bool)
	for _, rt := range vh.Routes {
		for _, matcher := range rt.Matchers {
			hash := hashutils.MustHash(matcher)
			if _, ok := seenMatchers[hash]; ok == true {
				reports.AddWarning(vs, ConflictingMatcherErr(vh.GetName(), matcher).Error())
			} else {
				seenMatchers[hash] = true
			}
		}
	}
}

func validateRouteOrder(vs *v1.VirtualService, vh *gloov1.VirtualHost, reports reporter.ResourceReports) {
	// warn on unordered routes
	var routesCopy []*gloov1.Route
	for _, rt := range vh.GetRoutes() {
		rtCopy := *rt
		routesCopy = append(routesCopy, &rtCopy)
	}

	utils.SortRoutesByPath(routesCopy)

	for idx, rt := range routesCopy {
		other := vh.GetRoutes()[idx]
		if !rt.Equal(other) {
			reports.AddWarning(vs, UnorderedRoutesErr(vh.GetName(), rt.GetName(), vh.GetRoutes()[idx].GetName(),
				utils.GetSmallestMatcher(rt.Matchers), utils.GetSmallestMatcher(other.Matchers)).Error())
		}
	}
}

func validateRegexHijacking(vs *v1.VirtualService, vh *gloov1.VirtualHost, reports reporter.ResourceReports) {
	// warn on early regex matchers that catch-all on later routes

	seenRegexMatchers := []string{}
	for _, rt := range vh.Routes {
		for _, matcher := range rt.Matchers {
			if matcher.GetRegex() != "" {
				seenRegexMatchers = append(seenRegexMatchers, matcher.GetRegex())
			} else {
				// make sure the current matcher doesn't match any previously defined regex.
				// this code is written with the assumption that the routes are already in their final order;
				// we are trying to help users avoid misconfiguration and short-circuiting errors
				path := utils.PathAsString(matcher)
				for _, regex := range seenRegexMatchers {
					re := regexp.MustCompile(regex)
					foundIndex := re.FindStringIndex(path)
					if foundIndex != nil {
						// we opt to warn on conflicting regexes even if the methods, query parameters, etc on the
						// "conflicting" matchers do not form a conflict because:
						//  - updating the regex to be less permissive is generally preferable to adding/removing methods/query parameter matchers
						//  - accounting for it would be more difficult to maintain as we add new matcher fields
						//  - such a conflict would be rare, and likely unintentional anyways
						reports.AddWarning(vs, UnorderedRegexErr(vh.GetName(), regex, matcher).Error())
					}
				}
			}
		}
	}
}
