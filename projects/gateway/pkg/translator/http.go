package translator

import (
	"context"
	"fmt"
	"strings"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/go-utils/hashutils"

	"github.com/solo-io/go-utils/errors"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/gogo/protobuf/proto"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var (
	NoVirtualHostErr = func(vs *v1.VirtualService) error {
		return errors.Errorf("virtual service [%s] does not specify a virtual host", vs.Metadata.Ref().Key())
	}
	DomainInOtherVirtualServicesErr = func(domain string, conflictingVsNames []string) error {
		if domain == "" {
			return errors.Errorf("domain conflict: other virtual services associated with the gateway that manages this "+
				"virtual service don't specify a domain (and thus default to '*'): %v", conflictingVsNames)
		}
		return errors.Errorf("domain conflict: the [%s] domain is present in other virtual services "+
			"associated with the gateway that manages this virtual service: %v", domain, conflictingVsNames)
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

// TODO(marco): validate SNI domains?
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

	// by default, virtual services live in the same namespace as the referencing gateway
	virtualServiceNamespaces := []string{gateway.Metadata.Namespace}

	if len(httpGateway.VirtualServiceNamespaces) > 0 {
		virtualServiceNamespaces = httpGateway.VirtualServiceNamespaces
	}

	for _, ns := range virtualServiceNamespaces {
		if ns == "*" || virtualService.Metadata.Namespace == ns {
			return true
		}
	}
	return false
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
	routes, err := convertRoutes(vs, tables, reports)
	if err != nil {
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

func convertRoutes(vs *v1.VirtualService, tables v1.RouteTableList, reports reporter.ResourceReports) ([]*gloov1.Route, error) {
	var routes []*gloov1.Route
	for _, r := range vs.GetVirtualHost().GetRoutes() {
		rv := &routeVisitor{tables: tables}
		mergedRoutes, err := rv.convertRoute(vs, r, reports)
		if err != nil {
			return nil, err
		}
		for _, route := range mergedRoutes {
			if err := appendSource(route, vs); err != nil {
				// should never happen
				return nil, err
			}
		}
		routes = append(routes, mergedRoutes...)
	}
	return routes, nil
}

// converts a tree of gateway Routes into a list of Gloo routes
type routeVisitor struct {
	ctx     context.Context
	tables  v1.RouteTableList
	visited v1.RouteTableList
}

func (rv *routeVisitor) convertRoute(ownerResource resources.InputResource, ours *v1.Route, reports reporter.ResourceReports) ([]*gloov1.Route, error) {
	matchers := []*matchers.Matcher{defaults.DefaultMatcher()}
	if len(ours.Matchers) > 0 {
		matchers = ours.Matchers
	}

	route := &gloov1.Route{
		Matchers: matchers,
		Options:  ours.Options,
	}
	switch action := ours.Action.(type) {
	case *v1.Route_RedirectAction:
		route.Action = &gloov1.Route_RedirectAction{
			RedirectAction: action.RedirectAction,
		}
	case *v1.Route_DirectResponseAction:
		route.Action = &gloov1.Route_DirectResponseAction{
			DirectResponseAction: action.DirectResponseAction,
		}
	case *v1.Route_RouteAction:
		route.Action = &gloov1.Route_RouteAction{
			RouteAction: action.RouteAction,
		}
	case *v1.Route_DelegateAction:
		return rv.convertDelegateAction(ownerResource, ours, reports)
	}
	return []*gloov1.Route{route}, nil
}

var (
	matcherCountErr     = errors.New("invalid route: routes with delegate actions must omit or specify a single matcher")
	missingPrefixErr    = errors.New("invalid route: routes with delegate actions must use a prefix matcher")
	invalidPrefixErr    = errors.New("invalid route: route table matchers must begin with the prefix of their parent route's matcher")
	hasHeaderMatcherErr = errors.New("invalid route: routes with delegate actions cannot use header matchers")
	hasMethodMatcherErr = errors.New("invalid route: routes with delegate actions cannot use method matchers")
	hasQueryMatcherErr  = errors.New("invalid route: routes with delegate actions cannot use query matchers")
	delegationCycleErr  = errors.New("invalid route: delegation cycle detected")

	noDelegateActionErr = errors.New("internal error: convertDelegateAction() called on route without delegate action")

	routeTableMissingWarning = func(ref core.ResourceRef) string {
		return fmt.Sprintf("route table %v.%v missing", ref.Namespace, ref.Name)
	}
	invalidRouteTableForDelegateErr = func(delegatePrefix, pathString string) error {
		return errors.Wrapf(invalidPrefixErr, "required prefix: %v, path: %v", delegatePrefix, pathString)
	}
)

func (rv *routeVisitor) convertDelegateAction(routingResource resources.InputResource, route *v1.Route, reports reporter.ResourceReports) ([]*gloov1.Route, error) {
	delegate := route.GetDelegateAction()
	if delegate == nil {
		return nil, noDelegateActionErr
	}

	delegatePrefix, err := getDelegateRoutePrefix(route)
	if err != nil {
		return nil, err
	}

	var routeTableRef core.ResourceRef
	// handle deprecated route table resource reference format
	// TODO: remove when we remove the deprecated fields from the API
	if delegate.Namespace != "" || delegate.Name != "" {
		routeTableRef = core.ResourceRef{
			Namespace: delegate.Namespace,
			Name:      delegate.Name,
		}
	} else {
		switch selectorType := delegate.GetDelegationType().(type) {
		case *v1.DelegateAction_Selector:
			// TODO(marco): handle selector
			return nil, errors.New("delegate action selectors are not implemented yet!")
		case *v1.DelegateAction_Ref:
			routeTableRef = *selectorType.Ref
		}
	}

	// missing refs should only result in a warning
	// this allows resources to be applied asynchronously
	routeTable, err := rv.tables.Find(routeTableRef.Strings())
	if err != nil {
		reports.AddWarning(routingResource, routeTableMissingWarning(routeTableRef))
		return nil, nil
	}

	for _, visited := range rv.visited {
		if routeTable == visited {
			return nil, delegationCycleErr
		}
	}

	subRv := &routeVisitor{tables: rv.tables, visited: []*v1.RouteTable{routeTable}}
	for _, vis := range rv.visited {
		subRv.visited = append(subRv.visited, vis)
	}

	plugins := route.GetOptions()

	var delegatedRoutes []*gloov1.Route
	for _, routeTableRoute := range routeTable.Routes {
		// clone route since we mutate
		routeTableRoute := proto.Clone(routeTableRoute).(*v1.Route)

		merged, err := mergeRoutePlugins(routeTableRoute.GetOptions(), plugins)
		if err != nil {
			// should never happen
			return nil, errors.Wrapf(err, "internal error: merging route plugins from parent to delegated route")
		}
		routeTableRoute.Options = merged

		err = isRouteTableValidForDelegatePrefix(delegatePrefix, routeTableRoute)
		if err != nil {
			reports.AddError(routingResource, err)
			continue
		}

		subRoutes, err := subRv.convertRoute(routeTable, routeTableRoute, reports)
		if err != nil {
			return nil, errors.Wrapf(err, "converting sub-route")
		}
		for _, sub := range subRoutes {
			if err := appendSource(sub, routeTable); err != nil {
				// should never happen
				return nil, err
			}
			delegatedRoutes = append(delegatedRoutes, sub)
		}
	}

	return delegatedRoutes, nil
}

func getDelegateRoutePrefix(route *v1.Route) (string, error) {
	switch len(route.GetMatchers()) {
	case 0:
		return defaults.DefaultMatcher().GetPrefix(), nil
	case 1:
		matcher := route.GetMatchers()[0]
		var prefix string
		if len(matcher.GetHeaders()) > 0 {
			return prefix, hasHeaderMatcherErr
		}
		if len(matcher.GetMethods()) > 0 {
			return prefix, hasMethodMatcherErr
		}
		if len(matcher.GetQueryParameters()) > 0 {
			return prefix, hasQueryMatcherErr
		}
		if matcher.GetPathSpecifier() == nil {
			return defaults.DefaultMatcher().GetPrefix(), nil // no path specifier provided, default to '/' prefix matcher
		}
		prefix = matcher.GetPrefix()
		if prefix == "" {
			return prefix, missingPrefixErr
		}
		return prefix, nil
	default:
		return "", matcherCountErr
	}
}

func isRouteTableValidForDelegatePrefix(delegatePrefix string, routeTable *v1.Route) error {
	for _, match := range routeTable.Matchers {
		// ensure all subroutes in the delegated route table match the parent prefix
		if pathString := glooutils.PathAsString(match); !strings.HasPrefix(pathString, delegatePrefix) {
			return invalidRouteTableForDelegateErr(delegatePrefix, pathString)
		}
	}
	return nil
}
