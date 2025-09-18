package waypoint

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/types/known/wrapperspb"
	authcr "istio.io/client-go/pkg/apis/security/v1"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/sets"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/sandwich"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/httproute"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/stringutils"
)

const (
	// IstioPROXYProtocol is the only protocol for a kgateway-waypoint's Listener
	IstioPROXYProtocol = "istio.io/PROXY"

	loopbackBindAddr   = "127.0.0.1"
	wildcardBindAddr   = "0.0.0.0"
	loopbackBindAddrV6 = "::ffff:127.0.0.1"
	wildcardBindAddrV6 = "::"
)

var logger = logging.New("plugin/waypoint")

type waypointTranslator struct {
	queries         query.GatewayQueries
	waypointQueries waypointquery.WaypointQueries

	localBind     bool
	rootNamespace string
	bindIpv6      bool
}

var _ extensionsplug.KGwTranslator = &waypointTranslator{}

func NewTranslator(
	queries query.GatewayQueries,
	waypointQueries waypointquery.WaypointQueries,
	settings settings.Settings,
) extensionsplug.KGwTranslator {
	return &waypointTranslator{
		queries:         queries,
		waypointQueries: waypointQueries,
		localBind:       settings.WaypointLocalBinding,
		rootNamespace:   settings.IstioNamespace,
		bindIpv6:        settings.ListenerBindIpv6,
	}
}

// Translate implements extensionsplug.KGwTranslator.
func (w *waypointTranslator) Translate(
	kctx krt.HandlerContext,
	ctx context.Context,
	gateway *ir.Gateway,
	reporter reports.Reporter,
) *ir.GatewayIR {
	gwReporter := reporter.Gateway(gateway.Obj)
	proxyListener, gwListener := w.buildInboundListener(gateway, gwReporter)
	if proxyListener == nil || gwListener == nil {
		// reporting/logging in BuildInboundListener
		return nil
	}

	// get the top level routes; these apply to every service that uses the waypoint
	// and get merged with per-service routes
	routes, err := w.fetchGatewayRoutes(kctx, ctx, gateway, gwListener, reporter, gwReporter)
	if err != nil {
		logger.Error("failed getting HTTPRoutes for Gateway", "error", err)
		return nil
	}

	// track unique attached routes for reporting
	// attachment happens at the Gateway/Listener level and at a per-service
	// level
	attachedRoutes := sets.New[types.NamespacedName]()
	for _, hr := range routes {
		attachedRoutes.Insert(namespacedName(hr))
	}

	waypointFor := waypointquery.GetWaypointFor(gateway.Obj)

	if waypointFor.ForService() {
		http, tcp := w.buildServiceChains(
			kctx,
			ctx,
			logger,
			reporter,
			gateway,
			routes,
			gwListener,
			attachedRoutes,
			w.rootNamespace,
		)
		proxyListener.HttpFilterChain = append(proxyListener.HttpFilterChain, http...)
		proxyListener.TcpFilterChain = append(proxyListener.TcpFilterChain, tcp...)
	}

	// ensure consistent ordering in outputs
	proxyListener.HttpFilterChain = slices.SortBy(proxyListener.HttpFilterChain, func(fc ir.HttpFilterChainIR) string {
		return fc.FilterChainName
	})
	proxyListener.TcpFilterChain = slices.SortBy(proxyListener.TcpFilterChain, func(fc ir.TcpIR) string {
		return fc.FilterChainName
	})

	// don't include the listener unless we have filter chains
	outListeners := []ir.ListenerIR{}
	if len(proxyListener.HttpFilterChain)+len(proxyListener.TcpFilterChain) > 0 {
		outListeners = append(outListeners, *proxyListener)
	}

	return &ir.GatewayIR{
		Listeners:            outListeners,
		SourceObject:         gateway,
		AttachedPolicies:     gateway.AttachedListenerPolicies,
		AttachedHttpPolicies: gateway.AttachedHttpPolicies,
	}
}

var waypointSupportedKinds = []gwv1.RouteGroupKind{
	{
		Group: ptr.To(gwv1.Group(gwv1.GroupName)),
		Kind:  wellknown.HTTPRouteKind,
	},
}

// TODO allow _not_ specifying any listeners and inferring the specific
// structure we expect with reasonable defaults (15088)
func (w *waypointTranslator) buildInboundListener(gw *ir.Gateway, reporter reports.GatewayReporter) (*ir.ListenerIR, *ir.Listener) {
	// find the single inbound listener
	var gatewayListener *ir.Listener
	for _, l := range gw.Listeners {
		if gatewayListener == nil && l.Protocol == IstioPROXYProtocol {
			gatewayListener = &l

			// supportedKinds union with  the allowed kinds
			// if no allowed kinds, just use all of our default supportedKinds
			supportedKinds := slices.Filter(waypointSupportedKinds, func(s gwv1.RouteGroupKind) bool {
				return l.AllowedRoutes == nil || nil != slices.FindFunc(l.AllowedRoutes.Kinds, func(lk gwv1.RouteGroupKind) bool {
					return cmputils.PointerValsEqual(lk.Group, s.Group) && lk.Kind == s.Kind
				})
			})
			reporter.Listener(&l.Listener).SetSupportedKinds(supportedKinds)
			continue
		}

		// non istio.io/PROXY listeners shouldn't have routes attached
		reporter.Listener(&l.Listener).SetSupportedKinds([]gwv1.RouteGroupKind{})
		reporter.Listener(&l.Listener).SetCondition(reports.ListenerCondition{
			Type:    gwv1.ListenerConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.ListenerReasonInvalid,
			Message: "Only a 'istio.io/PROXY' listener is allowed for kgateway-waypoint Gateways.",
		})
	}
	if gatewayListener == nil {
		reporter.SetCondition(reports.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: "Must contain at one listener with the protocol 'istio.io/PROXY'",
		})
		return nil, nil
	}

	bindAddr := wildcardBindAddr
	if w.bindIpv6 {
		bindAddr = wildcardBindAddrV6
	}
	if w.localBind {
		bindAddr = loopbackBindAddr
		if w.bindIpv6 {
			bindAddr = loopbackBindAddrV6
		}
	}

	return &ir.ListenerIR{
		Name:              "proxy_protocol_inbound",
		BindAddress:       bindAddr,
		BindPort:          uint32(gatewayListener.Port),
		PolicyAncestorRef: gatewayListener.PolicyAncestorRef,

		AttachedPolicies: ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{
				sandwich.SandwichedInboundGK: {{
					GroupKind: sandwich.SandwichedInboundGK,
					PolicyIr:  sandwich.SandwichedInboundPolicy{},
				}},
			},
		},
	}, gatewayListener
}

func (t *waypointTranslator) fetchGatewayRoutes(
	kctx krt.HandlerContext,
	ctx context.Context,
	gw *ir.Gateway,
	gwListener *ir.Listener,
	reporter reports.Reporter,
	gwReporter reports.GatewayReporter,
) ([]*query.RouteInfo, error) {
	gwRoutes, err := t.queries.GetRoutesForGateway(kctx, ctx, gw)
	if err != nil {
		gwReporter.SetCondition(reports.GatewayCondition{
			Type:    gwv1.GatewayConditionProgrammed,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.GatewayReasonInvalid,
			Message: "failed fetching routes for gateway",
		})
		return nil, err
	}
	for _, rErr := range gwRoutes.RouteErrors {
		reporter.Route(rErr.Route.GetSourceObject()).ParentRef(&rErr.ParentRef).SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  rErr.Error.Reason,
			Message: rErr.Error.Error(),
		})
	}
	routes := gwRoutes.GetListenerResult(gwListener.Parent, string(gwListener.Name))
	if routes == nil {
		// no routes for the single inbound PROXY listener
		return nil, nil
	}
	if err := routes.Error; err != nil {
		logger.Error("listener error when fetching HTTPRoutes for Gateway", "error", err)
		return nil, err
	}

	return routes.Routes, nil
}

func (t *waypointTranslator) buildServiceChains(
	kctx krt.HandlerContext,
	ctx context.Context,
	logger *slog.Logger,
	baseReporter reports.Reporter,
	gw *ir.Gateway,
	gwRoutes []*query.RouteInfo,
	gwListener *ir.Listener,
	attachedRoutes sets.Set[types.NamespacedName],
	rootNamespace string,
) ([]ir.HttpFilterChainIR, []ir.TcpIR) {
	var httpOut []ir.HttpFilterChainIR
	var tcpOut []ir.TcpIR
	// get attached services (istio.io/use-waypoint)
	services := t.waypointQueries.GetWaypointServices(kctx, ctx, gw.Obj)
	logger.Debug("attaching waypoint services", "gateway", namespacedName(gw).String(), "services", len(services))

	// Fetch Gateway (and GatewayClass) attached policies
	gwAuthzPolicies := t.waypointQueries.GetAuthorizationPoliciesForGateway(
		kctx,
		ctx,
		gw.Obj,
		rootNamespace,
	)

	// for each service:
	// * 1:1 Service port -> filter chain
	// For HTTP:
	// * shared virtualhost across all the per-port chains for HTTPRoute config
	// * if there are no HTTPRoutes, per-port virtualhosts that just forward
	//   traffic without modification
	// For TCP:
	// * Just forward traffic
	// * TODO TCPRoute

	for _, svc := range services {
		serviceSpecificPolicies := t.waypointQueries.GetAuthorizationPoliciesForService(kctx, ctx, &svc)

		// Combine with gateway policies (which serve as namespace-wide policies)
		combinedPolicies := make([]*authcr.AuthorizationPolicy, 0, len(gwAuthzPolicies)+len(serviceSpecificPolicies))

		combinedPolicies = append(combinedPolicies, gwAuthzPolicies...)
		combinedPolicies = append(combinedPolicies, serviceSpecificPolicies...)

		tcpRBAC, httpRBAC := BuildRBAC(combinedPolicies, gw.Obj, &svc)

		// get Service-specific routes
		httpRoutes := gwRoutes
		svcRoutes := t.waypointQueries.GetHTTPRoutesForService(kctx, ctx, &svc)
		for _, r := range svcRoutes {
			attachedRoutes.Insert(namespacedName(r))
			httpRoutes = append(httpRoutes, &r)
		}

		// build a single virtual host from HTTPRoutes
		// HTTPRoutes apply at the Service level, not the port
		// level so we don't need to generate this multiple times
		// TODO respect `port` on parentRef
		httpRoutesVirtualHost := t.buildHTTPVirtualHost(ctx, baseReporter, gwListener, svc, httpRoutes)

		for _, svcPort := range svc.Ports {
			filterChain, err := initServiceChain(svc, svcPort)
			if err != nil {
				// TODO if/when we support headless, initServiceChain should be infallible
				logger.Debug(
					"service had invalid or missing VIPs",
					"service",
					svc.GetName(),
					"namespace",
					svc.GetNamespace(),
					"addresses",
					svc.Addresses,
				)
				continue
			}

			if svcPort.IsHTTP() {
				virtualHostForPort := httpRoutesVirtualHost
				if httpRoutesVirtualHost == nil {
					// no routes, build a host with a default route
					// that just forwards traffic
					virtualHostForPort = buildDefaultToPortVirtualHost(svc, svcPort)
				}
				httpChain := ir.HttpFilterChainIR{
					FilterChainCommon: filterChain,
					Vhosts:            []*ir.VirtualHost{virtualHostForPort},
				}

				// Apply HTTP RBAC filters to this HTTP filter chain
				applyHTTPRBACFilters(&httpChain, httpRBAC)
				httpOut = append(httpOut, httpChain)
			} else {
				tcpChain := ir.TcpIR{
					FilterChainCommon: filterChain,
					BackendRefs:       []ir.BackendRefIR{svc.BackendRef(svcPort)},
				}

				// Apply TCP RBAC filters to this TCP filter chain
				applyTCPRBACFilters(&tcpChain, tcpRBAC)
				tcpOut = append(tcpOut, tcpChain)
			}
		}
	}
	return httpOut, tcpOut
}

func filterChainName(
	svc waypointquery.Service,
	port waypointquery.ServicePort,
) string {
	proto := "tcp"
	if port.IsHTTP() {
		proto = "http"
	}
	// TODO(peering) this probably should use non peered name/namespace
	name := fmt.Sprintf("fc_%s_%d_%s_%s", proto, port.Port, svc.GetName(), svc.GetNamespace())
	return stringutils.TruncateMaxLength(name, wellknown.EnvoyConfigNameMaxLen)
}

func initServiceChain(
	svc waypointquery.Service,
	port waypointquery.ServicePort,
) (ir.FilterChainCommon, error) {
	prefixRanges, err := svc.CidrRanges()
	if err != nil {
		return ir.FilterChainCommon{}, err
	}
	match := ir.FilterChainMatch{
		PrefixRanges:    prefixRanges,
		DestinationPort: &wrapperspb.UInt32Value{Value: uint32(port.Port)},
	}

	fcCommon := ir.FilterChainCommon{
		FilterChainName: filterChainName(svc, port),
		Matcher:         match,
	}
	return fcCommon, nil
}

// buildHTTPVirtualHost translates httpRoutes attached to the Service
// including those attached to the gateway, and builds a VirtualHost
// that can be used on any per-port filter chain for this service.
func (t *waypointTranslator) buildHTTPVirtualHost(
	ctx context.Context,
	baseReporter reports.Reporter,
	gwListener *ir.Listener,
	svc waypointquery.Service,
	httpRoutes []*query.RouteInfo,
) *ir.VirtualHost {
	if len(httpRoutes) == 0 {
		return nil
	}
	var translatedRoutes []ir.HttpRouteRuleMatchIR
	// TODO should we do any pre-processing to HTTPRoutes?
	// Something like default backendRefs if empty?
	for _, httpRoute := range httpRoutes {
		parentRefReporter := baseReporter.Route(httpRoute.Object.GetSourceObject()).ParentRef(&httpRoute.ParentRef)
		translatedRoutes = append(translatedRoutes, httproute.TranslateGatewayHTTPRouteRules(
			ctx,
			httpRoute,
			parentRefReporter,
			baseReporter,
		)...)
	}
	return &ir.VirtualHost{
		Name: stringutils.TruncateMaxLength(
			"http_routes_"+svc.GetName()+"_"+svc.GetNamespace(),
			wellknown.EnvoyConfigNameMaxLen,
		),
		Rules:    translatedRoutes,
		Hostname: "*",
		// TODO not sure how this works.. will this also have sectionname-less policies?
		// should this also have gateway targeted policies?
		AttachedPolicies: gwListener.AttachedPolicies,
	}
}

// buildDefaultToPortVirtualHost builds a VirtualHost with no routes/policy
// that will simply forward traffic to the same service port we matched for
// a per-port filter chain for a single service.
// TODO this could return multiple vhosts for ServiceEntry as ServiceEntry
// can supply multiple hostnames, which each map to a separate backend.
func buildDefaultToPortVirtualHost(
	svc waypointquery.Service,
	port waypointquery.ServicePort,
) *ir.VirtualHost {
	virtualHost := &ir.VirtualHost{
		// TODO for peering, this should be the _original_ name, not the effective name.
		Name:     svc.DefaultVHostName(port),
		Hostname: "*",
		Rules: []ir.HttpRouteRuleMatchIR{{
			Backends: []ir.HttpBackend{{
				Backend:          svc.BackendRef(port),
				AttachedPolicies: ir.AttachedPolicies{},
			}},
			MatchIndex: 0,
			Match: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/"),
				},
			},
		}},
	}

	return virtualHost
}

type namespaced interface {
	GetName() string
	GetNamespace() string
}

func namespacedName(o namespaced) types.NamespacedName {
	return types.NamespacedName{Name: o.GetName(), Namespace: o.GetNamespace()}
}
