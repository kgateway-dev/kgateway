package translator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type HttpTranslator struct{}

func (t *HttpTranslator) GenerateListeners(ctx context.Context, snap *v2.ApiSnapshot, filteredGateways []*v2.Gateway, resourceErrs reporter.ResourceErrors) []*gloov1.Listener {
	if len(snap.VirtualServices) == 0 {
		contextutils.LoggerFrom(ctx).Debugf("%v had no virtual services", snap.Hash())
		return nil
	}
	var result []*gloov1.Listener
	for _, gateway := range filteredGateways {
		if gateway.GetHttpGateway() == nil {
			continue
		}

		virtualServices := getVirtualServiceForGateway(gateway, snap.VirtualServices, resourceErrs)
		filtered := filterVirtualServiceForGateway(gateway, virtualServices)
		mergedVirtualServices := validateAndMergeVirtualServices(gateway, filtered, resourceErrs)
		listener := desiredListenerForHttp(gateway, mergedVirtualServices, snap.RouteTables, resourceErrs)
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

func validateAndMergeVirtualServices(gateway *v2.Gateway, virtualServices v1.VirtualServiceList, resourceErrs reporter.ResourceErrors) v1.VirtualServiceList {
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
			resourceErrs.AddError(gateway, fmt.Errorf("domain %s is present in more than one vservice set in this gateway", domain))
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
				resourceErrs.AddError(gateway, fmt.Errorf("more than one distinct ssl config is present in virtual service of these domains: %s", k))
			}

			havePlugins := vs.VirtualHost != nil &&
				vs.VirtualHost.VirtualHostPlugins != nil

			if vhostPlugins == nil {
				if havePlugins {
					vhostPlugins = vs.VirtualHost.VirtualHostPlugins
				}
			} else if havePlugins {
				resourceErrs.AddError(gateway, fmt.Errorf("more than one vhost plugin is present in virtual service of these domains: %s", k))
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

func getVirtualServiceForGateway(gateway *v2.Gateway, virtualServices v1.VirtualServiceList, resourceErrs reporter.ResourceErrors) v1.VirtualServiceList {
	httpGateway := gateway.GetHttpGateway()
	if httpGateway == nil {
		return nil
	}

	var virtualServicesForGateway v1.VirtualServiceList

	switch {
	case len(httpGateway.VirtualServiceSelector) > 0:
		// select virtual services by the label selector
		// must be in the same namespace as the Gateway
		selector := labels.SelectorFromSet(httpGateway.VirtualServiceSelector)

		virtualServices.Each(func(element *v1.VirtualService) {
			vsLabels := labels.Set(element.Metadata.Labels)
			if element.Metadata.Namespace == gateway.Metadata.Namespace && selector.Matches(vsLabels) {
				virtualServicesForGateway = append(virtualServicesForGateway, element)
			}
		})

	default:
		// use individual refs to collect virtual services
		virtualServiceRefs := httpGateway.VirtualServices

		// fall back to all virtual services in all watchNamespaces
		// TODO: make this all vs in a single namespace
		// https://github.com/solo-io/gloo/issues/1142
		if len(virtualServiceRefs) == 0 {
			for _, virtualService := range virtualServices {
				virtualServiceRefs = append(virtualServiceRefs, core.ResourceRef{
					Name:      virtualService.GetMetadata().Name,
					Namespace: virtualService.GetMetadata().Namespace,
				})
			}
		}

		for _, ref := range virtualServiceRefs {
			virtualService, err := virtualServices.Find(ref.Strings())
			if err != nil {
				resourceErrs.AddError(gateway, err)
				continue
			}
			virtualServicesForGateway = append(virtualServicesForGateway, virtualService)
		}
	}

	return virtualServicesForGateway
}

func filterVirtualServiceForGateway(gateway *v2.Gateway, virtualServices v1.VirtualServiceList) v1.VirtualServiceList {
	var virtualServicesForGateway v1.VirtualServiceList
	for _, virtualService := range virtualServices {
		if gateway.Ssl == hasSsl(virtualService) {
			virtualServicesForGateway = append(virtualServicesForGateway, virtualService)
		}
	}
	return virtualServicesForGateway
}

func hasSsl(vs *v1.VirtualService) bool {
	return vs.SslConfig != nil
}

func desiredListenerForHttp(gateway *v2.Gateway, virtualServicesForGateway v1.VirtualServiceList, tables v1.RouteTableList, resourceErrs reporter.ResourceErrors) *gloov1.Listener {
	var (
		virtualHosts []*gloov1.VirtualHost
		sslConfigs   []*gloov1.SslConfig
	)

	for _, virtualService := range virtualServicesForGateway.Sort() {
		if virtualService.VirtualHost == nil {
			virtualService.VirtualHost = &v1.VirtualHost{}
		}
		vh, err := virtualServiceToVirtualHost(virtualService, tables, resourceErrs)
		if err != nil {
			resourceErrs.AddError(virtualService, err)
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

func virtualServiceToVirtualHost(vs *v1.VirtualService, tables v1.RouteTableList, resourceErrs reporter.ResourceErrors) (*gloov1.VirtualHost, error) {
	routes, err := convertRoutes(vs, tables, resourceErrs)
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

func convertRoutes(vs *v1.VirtualService, tables v1.RouteTableList, resourceErrs reporter.ResourceErrors) ([]*gloov1.Route, error) {
	var routes []*gloov1.Route
	for _, r := range vs.GetVirtualHost().GetRoutes() {
		rv := &routeVisitor{tables: tables}
		mergedRoutes, err := rv.convertRoute(vs, r, resourceErrs)
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

func (rv *routeVisitor) convertRoute(ownerResource resources.InputResource, ours *v1.Route, resourceErrs reporter.ResourceErrors) ([]*gloov1.Route, error) {
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
		return rv.convertDelegateAction(ownerResource, ours, resourceErrs)
	}
	return []*gloov1.Route{route}, nil
}

var (
	missingPrefixErr    = errors.Errorf("invalid route: routes with delegate actions must specify a prefix matcher")
	hasHeaderMatcherErr = errors.Errorf("invalid route: routes with delegate actions cannot use header matchers")
	hasMethodMatcherErr = errors.Errorf("invalid route: routes with delegate actions cannot use method matchers")
	hasQueryMatcherErr  = errors.Errorf("invalid route: routes with delegate actions cannot use query matchers")
	delegationCycleErr  = errors.Errorf("invalid route: delegation cycle detected")

	noDelegateActionErr = errors.Errorf("internal error: convertDelegateAction() called on route without delegate action")
)

func (rv *routeVisitor) convertDelegateAction(sourceResource resources.InputResource, ours *v1.Route, resourceErrs reporter.ResourceErrors) ([]*gloov1.Route, error) {
	action := ours.GetDelegateAction()
	if action == nil {
		return nil, noDelegateActionErr
	}

	matcher := ours.GetMatcher()

	prefix := matcher.GetPrefix()
	if prefix == "" {
		return nil, missingPrefixErr
	}
	prefix = strings.TrimSuffix("/"+strings.Trim(prefix, "/"), "/")

	if len(matcher.GetHeaders()) > 0 {
		return nil, hasHeaderMatcherErr
	}
	if len(matcher.GetMethods()) > 0 {
		return nil, hasMethodMatcherErr
	}
	if len(matcher.GetQueryParameters()) > 0 {
		return nil, hasQueryMatcherErr
	}
	routeTable, err := rv.tables.Find(action.Strings())
	if err != nil {
		resourceErrs.AddError(sourceResource, err)
		return nil, err
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
	var delegatedRoutes []*gloov1.Route
	for _, route := range routeTable.Routes {
		route := proto.Clone(route).(*v1.Route)
		subRoutes, err := subRv.convertRoute(routeTable, route, resourceErrs)
		if err != nil {
			return nil, errors.Wrapf(err, "converting sub-route")
		}
		for _, sub := range subRoutes {
			switch path := sub.Matcher.PathSpecifier.(type) {
			case *gloov1.Matcher_Exact:
				path.Exact = prefix + "/" + strings.TrimPrefix(path.Exact, "/")
			case *gloov1.Matcher_Regex:
				path.Regex = prefix + "/" + strings.TrimPrefix(path.Regex, "/")
			case *gloov1.Matcher_Prefix:
				path.Prefix = prefix + "/" + strings.TrimPrefix(path.Prefix, "/")
			}
			// inherit route plugins from parent
			if sub.RoutePlugins == nil {
				sub.RoutePlugins = proto.Clone(ours.RoutePlugins).(*gloov1.RoutePlugins)
			}
			if err := appendSource(sub, routeTable); err != nil {
				// should never happen
				return nil, err
			}
			delegatedRoutes = append(delegatedRoutes, sub)
		}
	}

	return delegatedRoutes, nil
}
