package listener

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
	"strings"

	"github.com/solo-io/gloo/projects/gateway2/translator/sslutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/ports"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/routeutils"
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type SslConfig struct {
	Bundle     TlsBundle
	SniDomains []string
}
type TlsBundle struct {
	CA         []byte
	PrivateKey []byte
	CertChain  []byte
}

type FilterChainInfo struct {
	SslConfig *SslConfig
}

type ListenerAndRoutes struct {
	Listener     *listenerv3.Listener          `json:"listener"`
	RouteConfigs []*routev3.RouteConfiguration `json:"route_configs"`
}

// TranslateListeners translates the set of gloo listeners required to produce a full output proxy (either form one Gateway or multiple merged Gateways)
func TranslateListeners(
	ctx context.Context,
	queries query.GatewayQueries,
	plugins registry.HTTPFilterPluginRegistry,
	gateway *gwv1.Gateway,
	routesForGw query.RoutesForGwResult,
	reporter reports.Reporter,
) []ListenerAndRoutes {
	validatedListeners := validateListeners(gateway, reporter.Gateway(gateway))

	mergedListeners := mergeGWListeners(queries, gateway.Namespace, validatedListeners, routesForGw, reporter.Gateway(gateway))
	translatedListeners := mergedListeners.translateListeners(ctx, plugins, queries, reporter)
	return translatedListeners
}

func mergeGWListeners(
	queries query.GatewayQueries,
	gatewayNamespace string,
	listeners []gwv1.Listener,
	routesForGw query.RoutesForGwResult,
	reporter reports.GatewayReporter,
) *mergedListeners {
	ml := &mergedListeners{
		gatewayNamespace: gatewayNamespace,
		queries:          queries,
	}
	for _, listener := range listeners {
		result, ok := routesForGw.ListenerResults[string(listener.Name)]
		if !ok || result.Error != nil {
			// TODO report
			// TODO, if Error is not nil, this is a user-config error on selectors
			// continue
		}
		listenerReporter := reporter.Listener(&listener)
		var routes []*query.ListenerRouteResult
		if result != nil {
			routes = result.Routes
		}
		ml.append(listener, routes, listenerReporter)
	}
	return ml
}

type mergedListeners struct {
	gatewayNamespace string
	listeners        []*mergedListener
	queries          query.GatewayQueries
}

func (ml *mergedListeners) append(
	listener gwv1.Listener,
	routes []*query.ListenerRouteResult,
	reporter reports.ListenerReporter,
) error {
	switch listener.Protocol {
	case gwv1.HTTPProtocolType:
		ml.appendHttpListener(listener, routes, reporter)
	case gwv1.HTTPSProtocolType:
		ml.appendHttpsListener(listener, routes, reporter)
	// TODO default handling
	default:
		return eris.Errorf("unsupported protocol: %v", listener.Protocol)
	}

	return nil
}

func (ml *mergedListeners) appendHttpListener(
	listener gwv1.Listener,
	routesWithHosts []*query.ListenerRouteResult,
	reporter reports.ListenerReporter,
) {
	parent := httpFilterChainParent{
		gatewayListenerName: string(listener.Name),
		routes:              routesWithHosts,
	}

	fc := &httpFilterChain{
		parents: []httpFilterChainParent{parent},
		queries: ml.queries,
	}
	listenerName := string(listener.Name)
	finalPort := gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port)))

	for _, lis := range ml.listeners {
		if lis.port == finalPort {
			// concatenate the names on the parent output listener/filterchain
			// TODO is this valid listener name?
			lis.name += "~" + listenerName
			if lis.httpFilterChain != nil {
				lis.httpFilterChain.parents = append(lis.httpFilterChain.parents, parent)
			} else {
				lis.httpFilterChain = fc
			}
			return
		}
	}

	// create a new filter chain for the listener
	ml.listeners = append(ml.listeners, &mergedListener{
		name:             listenerName,
		gatewayNamespace: ml.gatewayNamespace,
		port:             finalPort,
		httpFilterChain:  fc,
		listenerReporter: reporter,
	})

}

func (ml *mergedListeners) appendHttpsListener(
	listener gwv1.Listener,
	routesWithHosts []*query.ListenerRouteResult,
	reporter reports.ListenerReporter,
) {

	// create a new filter chain for the listener
	//protocol:            listener.Protocol,
	mfc := httpsFilterChain{
		gatewayListenerName: string(listener.Name),
		sniDomain:           listener.Hostname,
		tls:                 listener.TLS,
		routesWithHosts:     routesWithHosts,
		queries:             ml.queries,
	}

	listenerName := string(listener.Name)
	for _, lis := range ml.listeners {
		if lis.port == listener.Port {
			// concatenate the names on the parent output listener
			// TODO is this valid listener name?
			lis.name += "~" + listenerName
			lis.httpsFilterChains = append(lis.httpsFilterChains, mfc)
			return
		}
	}
	ml.listeners = append(ml.listeners, &mergedListener{
		name:              listenerName,
		gatewayNamespace:  ml.gatewayNamespace,
		port:              gwv1.PortNumber(ports.TranslatePort(uint16(listener.Port))),
		httpsFilterChains: []httpsFilterChain{mfc},
		listenerReporter:  reporter,
	})
}

func (ml *mergedListeners) translateListeners(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	queries query.GatewayQueries,
	reporter reports.Reporter) []ListenerAndRoutes {
	var listeners []ListenerAndRoutes
	for _, mergedListener := range ml.listeners {
		listener := mergedListener.translateListener(ctx, plugins, queries, reporter)
		listeners = append(listeners, listener)
	}
	return listeners
}

type mergedListener struct {
	name              string
	gatewayNamespace  string
	port              gwv1.PortNumber
	httpFilterChain   *httpFilterChain
	httpsFilterChains []httpsFilterChain
	listenerReporter  reports.ListenerReporter
	// TODO(policy via http listener options)
}

func RouteConfigName(ln string) string {
	return ln + "-routes"
}

func MatchedRouteConfigName(ln string, matcher *FilterChainInfo) string {
	namePrefix := RouteConfigName(ln)

	if matcher == nil {
		return namePrefix
	}
	// TODO do this better. make sure we don't hash anything secret as FNV is not a secure hash
	hash := fnv.New64()
	for _, s := range matcher.SslConfig.SniDomains {
		hash.Write([]byte(s))
	}

	nameSuffix := hash.Sum64()
	return fmt.Sprintf("%s-%d", namePrefix, nameSuffix)
}

func (ml *mergedListener) translateListener(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) ListenerAndRoutes {
	res := ListenerAndRoutes{
		Listener: &listenerv3.Listener{
			Name:    ml.name,
			Address: computeListenerAddress("::", uint32(ml.port)),
		},
	}

	if ml.httpFilterChain != nil {
		httpFilterChain, vhostsForFilterchain := ml.httpFilterChain.translateHttpFilterChain(
			ctx,
			ml.name,
			ml.gatewayNamespace,
			plugins,
			reporter,
		)

		routeConfigName := MatchedRouteConfigName(ml.name, httpFilterChain)
		rc := newRouteConfig(routeConfigName, vhostsForFilterchain)
		hcm := initializeHCM(routeConfigName)
		hcm.HttpFilters = computeHttpFilters()
		res.RouteConfigs = append(res.RouteConfigs, rc)

		res.Listener.FilterChains = append(res.Listener.FilterChains, makeFilterChain(httpFilterChain, hcm))

	}
	for _, mfc := range ml.httpsFilterChains {
		// each virtual host name must be unique across all filter chains
		// to prevent collisions because the vhosts have to be re-computed for each set
		httpsFilterChain, vhostsForFilterchain := mfc.translateHttpsFilterChain(
			ctx,
			plugins,
			mfc.gatewayListenerName,
			ml.gatewayNamespace,
			queries,
			reporter,
			ml.listenerReporter,
		)
		if httpsFilterChain == nil {
			// TODO report
			continue
		}

		routeConfigName := MatchedRouteConfigName(ml.name, httpsFilterChain)
		rc := newRouteConfig(routeConfigName, vhostsForFilterchain)
		hcm := initializeHCM(routeConfigName)
		hcm.HttpFilters = computeHttpFilters()
		res.RouteConfigs = append(res.RouteConfigs, rc)

		res.Listener.FilterChains = append(res.Listener.FilterChains, makeFilterChain(httpsFilterChain, hcm))

	}

	return res
}

// httpFilterChain each one represents a GW Listener that has been merged into a single Gloo Listener (with distinct filter chains).
// In the case where no GW Listener merging takes place, every listener will use a Gloo AggregatedListeener with 1 HTTP filter chain.
type httpFilterChain struct {
	parents []httpFilterChainParent
	queries query.GatewayQueries
}

type httpFilterChainParent struct {
	gatewayListenerName string
	routes              []*query.ListenerRouteResult
}

func (mfc *httpFilterChain) translateHttpFilterChain(
	ctx context.Context,
	parentName string,
	gatewayNamespace string,
	plugins registry.HTTPFilterPluginRegistry,
	reporter reports.Reporter,
) (*FilterChainInfo, []*routev3.VirtualHost) {

	var (
		routesByHost = map[string]routeutils.SortableRoutes{}
	)
	for _, parent := range mfc.parents {
		for _, routeWithHosts := range parent.routes {
			processRoutesWithHosts(ctx, routeWithHosts, routesByHost, reporter, plugins, mfc.queries)
		}
	}

	return nil, getHosts(parentName, routesByHost)
}

// httpFilterChain each one represents a GW Listener that has been merged into a single Gloo Listener (with distinct filter chains).
// In the case where no GW Listener merging takes place, every listener will use a Gloo AggregatedListeener with 1 HTTP filter chain.
type httpsFilterChain struct {
	gatewayListenerName string
	sniDomain           *gwv1.Hostname
	tls                 *gwv1.GatewayTLSConfig
	routesWithHosts     []*query.ListenerRouteResult
	queries             query.GatewayQueries
}

func (mfc *httpsFilterChain) translateHttpsFilterChain(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	parentName string,
	gatewayNamespace string,
	queries query.GatewayQueries,
	reporter reports.Reporter,
	listenerReporter reports.ListenerReporter,
) (*FilterChainInfo, []*routev3.VirtualHost) {
	// process routes first, so any route related errors are reported on the httproute.
	var (
		routesByHost = map[string]routeutils.SortableRoutes{}
	)
	for _, routeWithHosts := range mfc.routesWithHosts {
		processRoutesWithHosts(ctx, routeWithHosts, routesByHost, reporter, plugins, mfc.queries)
	}

	sslConfig, err := translateSslConfig(
		ctx,
		gatewayNamespace,
		mfc.sniDomain,
		mfc.tls,
		queries,
	)
	if err != nil {
		reason := gwv1.ListenerReasonRefNotPermitted
		if !errors.Is(err, query.ErrMissingReferenceGrant) {
			reason = gwv1.ListenerReasonInvalidCertificateRef
		}
		listenerReporter.SetCondition(reports.ListenerCondition{
			Type:   gwv1.ListenerConditionResolvedRefs,
			Status: metav1.ConditionFalse,
			Reason: reason,
		})
		// listener with no ssl is invalid. We return nil so set programmed to false
		listenerReporter.SetCondition(reports.ListenerCondition{
			Type:   gwv1.ListenerConditionProgrammed,
			Status: metav1.ConditionFalse,
			Reason: gwv1.ListenerReasonInvalid,
		})
		return nil, nil
	}

	virtualHosts := make([]*routev3.VirtualHost, 0, len(routesByHost))
	for host, vhostRoutes := range routesByHost {
		sort.Stable(vhostRoutes)
		vhostName := makeVhostName(parentName, host)
		virtualHosts = append(virtualHosts, &routev3.VirtualHost{
			Name:    vhostName,
			Domains: []string{host},
			Routes:  vhostRoutes.ToRoutes(),
		})
	}
	slices.SortFunc(virtualHosts, func(a, b *routev3.VirtualHost) int {
		return strings.Compare(a.Name, b.Name)
	})

	return &FilterChainInfo{
		SslConfig: sslConfig, // http filter chain matcher is not used
	}, getHosts(parentName, routesByHost)
}

func processRoutesWithHosts(ctx context.Context, routeWithHosts *query.ListenerRouteResult, routesByHost map[string]routeutils.SortableRoutes,
	reporter reports.Reporter,
	plugins registry.HTTPFilterPluginRegistry,
	queries query.GatewayQueries,

) {
	parentRefReporter := reporter.Route(&routeWithHosts.Route).ParentRef(&routeWithHosts.ParentRef)
	routes := httproute.TranslateGatewayHTTPRouteRules(
		ctx,
		plugins,
		queries,
		routeWithHosts.Route,
		parentRefReporter,
	)

	if len(routes) == 0 {
		// TODO report
		return
	}

	hostnames := routeWithHosts.Hostnames
	if len(hostnames) == 0 {
		hostnames = []string{"*"}
	}

	for _, host := range hostnames {
		routesByHost[host] = append(routesByHost[host], routes...)
	}
}

func getHosts(
	parentName string,
	routesByHost map[string]routeutils.SortableRoutes) []*routev3.VirtualHost {
	virtualHosts := make([]*routev3.VirtualHost, 0, len(routesByHost))
	for host, vhostRoutes := range routesByHost {
		sort.Stable(vhostRoutes)
		vhostName := makeVhostName(parentName, host)
		virtualHosts = append(virtualHosts, &routev3.VirtualHost{
			Name:    vhostName,
			Domains: []string{host},
			Routes:  vhostRoutes.ToRoutes(),
		})
	}
	slices.SortFunc(virtualHosts, func(a, b *routev3.VirtualHost) int {
		return strings.Compare(a.Name, b.Name)
	})
	return virtualHosts
}

func translateSslConfig(
	ctx context.Context,
	parentNamespace string,
	sniDomain *gwv1.Hostname,
	tls *gwv1.GatewayTLSConfig,
	queries query.GatewayQueries,
) (*SslConfig, error) {
	if tls == nil {
		return nil, nil
	}

	// TODO support passthrough mode
	if tls.Mode == nil ||
		*tls.Mode != gwv1.TLSModeTerminate {
		return nil, nil
	}

	var k8sSecretToUse *corev1.Secret
	for _, certRef := range tls.CertificateRefs {
		// validate via query
		secret, err := queries.GetSecretForRef(ctx, query.FromGkNs{
			Gk: metav1.GroupKind{
				Group: gwv1.GroupName,
				Kind:  "Gateway",
			},
			Ns: parentNamespace,
		}, certRef)
		if err != nil {
			return nil, err
		}
		k8sSecret, ok := secret.(*corev1.Secret)
		if !ok {
			return nil, fmt.Errorf("error: secret %v (type %T) is not a kubernetes secret", certRef.Name, secret)
		}
		if err := sslutils.ValidateTlsSecret(k8sSecret); err != nil {
			return nil, err
		}

		k8sSecretToUse = k8sSecret
		break // TODO support multiple certs
	}
	if k8sSecretToUse == nil {
		return nil, nil
	}

	var sniDomains []string
	if sniDomain != nil {
		sniDomains = []string{string(*sniDomain)}
	}
	return &SslConfig{
		Bundle: TlsBundle{
			CA:         k8sSecretToUse.Data[corev1.ServiceAccountRootCAKey],
			PrivateKey: k8sSecretToUse.Data[corev1.TLSPrivateKeyKey],
			CertChain:  k8sSecretToUse.Data[corev1.TLSCertKey],
		},
		SniDomains: sniDomains,
	}, nil
}

// makeVhostName computes the name of a virtual host based on the parent name and domain.
func makeVhostName(
	parentName string,
	domain string,
) string {
	// TODO is this a valid vh name?
	return parentName + "~" + domain
}
