package translator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/solo-io/go-utils/errors"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/gogo/protobuf/proto"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type HttpTranslator struct{}

func (t *HttpTranslator) GenerateListeners(ctx context.Context, snap *v2.ApiSnapshot, filteredGateways []*v2.Gateway, reports reporter.ResourceReports) []*gloov1.Listener {
	if len(snap.VirtualServices) == 0 {
		contextutils.LoggerFrom(ctx).Debugf("%v had no virtual services", snap.Hash())
		return nil
	}
	var result []*gloov1.Listener
	for _, gateway := range filteredGateways {
		if gateway.GetHttpGateway() == nil {
			continue
		}

		virtualServices := getVirtualServicesForGateway(gateway, snap.VirtualServices)
		mergedVirtualServices := validateAndMergeVirtualServices(gateway, virtualServices, reports)
		listener := desiredListenerForHttp(gateway, mergedVirtualServices, snap.RouteTables, reports)
		result = append(result, listener)
	}
	return result
}

func domainsToKey(domains []string) string {
	// copy before mutating for good measure
	domains = append([]string{}, domains...)
	// sort, and join all domains with an out of band character, like ','
	sort.Strings(domains)
	return strings.Join(domains, ",")
}

func validateAndMergeVirtualServices(gateway *v2.Gateway, virtualServices v1.VirtualServiceList, reports reporter.ResourceReports) v1.VirtualServiceList {
	ns := gateway.Metadata.GetNamespace()
	domainKeysSets := map[string]v1.VirtualServiceList{}
	for _, vs := range virtualServices {
		if vs.VirtualHost == nil {
			continue
		}
		domainsKey := domainsToKey(vs.VirtualHost.Domains)
		domainKeysSets[domainsKey] = append(domainKeysSets[domainsKey], vs)
	}

	domainSet := map[string][]string{}
	// make sure each domain is only in one domain set
	for k, vslist := range domainKeysSets {
		// take the first one as they are all the same
		domains := vslist[0].VirtualHost.Domains
		for _, d := range domains {
			domainSet[d] = append(domainSet[d], k)
		}
	}

	// report errors
	for domain, domainSetKeys := range domainSet {
		if len(domainSetKeys) > 1 {
			reports.AddError(gateway, fmt.Errorf("domain %s is present in more than one vservice set in this gateway", domain))
		}
	}
	// return merged list
	var mergedVirtualServices v1.VirtualServiceList
	for k, vslist := range domainKeysSets {
		if len(vslist) == 1 {
			// only one vservice, do nothing.
			mergedVirtualServices = append(mergedVirtualServices, vslist[0])
			continue
		}

		// take the first one as they are all the same
		var routes []*v1.Route
		var sslConfig *gloov1.SslConfig
		var vhostPlugins *gloov1.VirtualHostPlugins
		for _, vs := range vslist {
			routes = append(routes, vs.VirtualHost.Routes...)
			if sslConfig == nil {
				sslConfig = vs.SslConfig
			} else if !vs.SslConfig.Equal(sslConfig) {
				reports.AddError(gateway, fmt.Errorf("more than one distinct ssl config is present in virtual service of these domains: %s", k))
			}

			havePlugins := vs.VirtualHost != nil &&
				vs.VirtualHost.VirtualHostPlugins != nil

			if vhostPlugins == nil {
				if havePlugins {
					vhostPlugins = vs.VirtualHost.VirtualHostPlugins
				}
			} else if havePlugins {
				reports.AddError(gateway, fmt.Errorf("more than one vhost plugin is present in virtual service of these domains: %s", k))
			}
		}

		glooutils.SortGatewayRoutesByPath(routes)

		ref := core.Metadata{
			// name shouldnt matter as it this object is ephemeral.
			Name:      getMergedName(k),
			Namespace: ns,
		}
		mergedVs := &v1.VirtualService{
			VirtualHost: &v1.VirtualHost{
				Domains:            vslist[0].VirtualHost.Domains,
				Routes:             routes,
				VirtualHostPlugins: vhostPlugins,
			},
			SslConfig: sslConfig,
			Metadata:  ref,
		}
		mergedVirtualServices = append(mergedVirtualServices, mergedVs)
	}

	return mergedVirtualServices
}

func getMergedName(k string) string {
	if k == "" {
		return "catchall"
	}
	return "merged-" + k
}

func getVirtualServicesForGateway(gateway *v2.Gateway, virtualServices v1.VirtualServiceList) v1.VirtualServiceList {

	var virtualServicesForGateway v1.VirtualServiceList
	for _, vs := range virtualServices {
		if GatewayContainsVirtualService(gateway, vs) {
			virtualServicesForGateway = append(virtualServicesForGateway, vs)
		}
	}

	return virtualServicesForGateway
}

func GatewayContainsVirtualService(gateway *v2.Gateway, virtualService *v1.VirtualService) bool {
	httpGateway := gateway.GetHttpGateway()
	if httpGateway == nil {
		return false
	}

	if gateway.Ssl != hasSsl(virtualService) {
		return false
	}

	if len(httpGateway.VirtualServiceSelector) > 0 {
		// select virtual services by the label selector
		// must be in the same namespace as the Gateway
		selector := labels.SelectorFromSet(httpGateway.VirtualServiceSelector)

		vsLabels := labels.Set(virtualService.Metadata.Labels)

		// must match both labels and namespace
		return virtualService.Metadata.Namespace == gateway.Metadata.Namespace && selector.Matches(vsLabels)
	}
	// use individual refs to collect virtual services
	virtualServiceRefs := httpGateway.VirtualServices

	if len(virtualServiceRefs) == 0 {
		// fall back to all virtual services in all watchNamespaces
		// TODO: make this all vs in a single namespace
		// https://github.com/solo-io/gloo/issues/1142
		return true
	}

	vsRef := virtualService.Metadata.Ref()

	for _, ref := range virtualServiceRefs {
		if ref == vsRef {
			return true
		}
	}

	return false
}

func hasSsl(vs *v1.VirtualService) bool {
	return vs.SslConfig != nil
}

func desiredListenerForHttp(gateway *v2.Gateway, virtualServicesForGateway v1.VirtualServiceList, tables v1.RouteTableList, reports reporter.ResourceReports) *gloov1.Listener {
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

	var httpPlugins *gloov1.HttpListenerPlugins
	if httpGateway := gateway.GetHttpGateway(); httpGateway != nil {
		httpPlugins = httpGateway.Plugins
	}
	listener := standardListener(gateway)
	listener.ListenerType = &gloov1.Listener_HttpListener{
		HttpListener: &gloov1.HttpListener{
			VirtualHosts:    virtualHosts,
			ListenerPlugins: httpPlugins,
		},
	}
	listener.SslConfigurations = sslConfigs
	return listener
}

func virtualServiceToVirtualHost(vs *v1.VirtualService, tables v1.RouteTableList, reports reporter.ResourceReports) (*gloov1.VirtualHost, error) {
	routes, err := convertRoutes(vs, tables, reports)
	if err != nil {
		return nil, err
	}

	vh := &gloov1.VirtualHost{
		Name:               fmt.Sprintf("%v.%v", vs.Metadata.Namespace, vs.Metadata.Name),
		Domains:            vs.VirtualHost.Domains,
		Routes:             routes,
		VirtualHostPlugins: vs.VirtualHost.VirtualHostPlugins,
		// TODO: remove on next breaking change
		CorsPolicy: vs.VirtualHost.CorsPolicy,
	}

	return vh, nil
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
	route := &gloov1.Route{
		Matcher:      ours.Matcher,
		RoutePlugins: ours.RoutePlugins,
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
	missingPrefixErr    = errors.New("invalid route: routes with delegate actions must specify a prefix matcher")
	invalidPrefixErr    = errors.New("invalid route: routes within the route table must begin with the prefix of their parent")
	invalidMatcherErr   = errors.New("invalid route: matcher cannot be missing")
	hasHeaderMatcherErr = errors.New("invalid route: routes with delegate actions cannot use header matchers")
	hasMethodMatcherErr = errors.New("invalid route: routes with delegate actions cannot use method matchers")
	hasQueryMatcherErr  = errors.New("invalid route: routes with delegate actions cannot use query matchers")
	delegationCycleErr  = errors.New("invalid route: delegation cycle detected")

	noDelegateActionErr = errors.New("internal error: convertDelegateAction() called on route without delegate action")

	routeTableMissingWarning = func(ref core.ResourceRef) string {
		return fmt.Sprintf("route table %v.%v missing", ref.Namespace, ref.Name)
	}
)

func (rv *routeVisitor) convertDelegateAction(routingResource resources.InputResource, route *v1.Route, reports reporter.ResourceReports) ([]*gloov1.Route, error) {
	delegate := route.GetDelegateAction()
	if delegate == nil {
		return nil, noDelegateActionErr
	}

	matcher := route.GetMatcher()

	prefix := matcher.GetPrefix()
	if prefix == "" {
		return nil, missingPrefixErr
	}

	if len(matcher.GetHeaders()) > 0 {
		return nil, hasHeaderMatcherErr
	}
	if len(matcher.GetMethods()) > 0 {
		return nil, hasMethodMatcherErr
	}
	if len(matcher.GetQueryParameters()) > 0 {
		return nil, hasQueryMatcherErr
	}

	// missing refs should only result in a warning
	// this allows resources to be applied asynchronously
	routeTable, err := rv.tables.Find(delegate.Strings())
	if err != nil {
		reports.AddWarning(routingResource, routeTableMissingWarning(*delegate))
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

	plugins := route.GetRoutePlugins()

	var delegatedRoutes []*gloov1.Route
	for _, routeTableRoute := range routeTable.Routes {
		// clone route since we mutate
		routeTableRoute := proto.Clone(routeTableRoute).(*v1.Route)

		merged, err := mergeRoutePlugins(routeTableRoute.GetRoutePlugins(), plugins)
		if err != nil {
			// should never happen
			return nil, errors.Wrapf(err, "internal error: merging route plugins from parent to delegated route")
		}
		routeTableRoute.RoutePlugins = merged

		match := routeTableRoute.GetMatcher()
		if match == nil {
			reports.AddError(routingResource, invalidMatcherErr)
			continue
		}

		// ensure all subroutes in the delegated route table match the parent prefix
		if pathString := glooutils.PathAsString(match); !strings.HasPrefix(pathString, prefix) {
			err = errors.Wrapf(invalidPrefixErr, "required prefix: %v, path: %v", prefix, pathString)
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
