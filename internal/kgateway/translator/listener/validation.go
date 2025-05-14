package listener

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

const NormalizedHTTPSTLSType = "HTTPS/TLS"
const DefaultHostname = "*"

type portProtocol struct {
	hostnames map[gwv1.Hostname]int
	protocol  map[gwv1.ProtocolType]bool
	// needed for getting reporter? doesn't seem great
	listeners []ir.Listener
}

type protocol = string
type groupName = string
type routeKind = string

// getSupportedProtocolsRoutes returns a map of listener protocols to the supported route kinds for that protocol
func getSupportedProtocolsRoutes() map[protocol]map[groupName][]routeKind {
	supportedProtocolToKinds := map[protocol]map[groupName][]routeKind{
		string(gwv1.HTTPProtocolType): {
			gwv1.GroupName: []string{
				wellknown.HTTPRouteKind,
				wellknown.GRPCRouteKind,
			},
		},
		string(gwv1.HTTPSProtocolType): {
			gwv1.GroupName: []string{
				wellknown.HTTPRouteKind,
			},
		},
		string(gwv1.TCPProtocolType): {
			gwv1.GroupName: []string{
				wellknown.TCPRouteKind,
			},
		},
		string(gwv1.TLSProtocolType): {
			gwv1.GroupName: []string{
				wellknown.TLSRouteKind,
			},
		},
	}
	return supportedProtocolToKinds
}

func buildDefaultRouteKindsForProtocol(supportedRouteKindsForProtocol map[groupName][]routeKind) []gwv1.RouteGroupKind {
	rgks := []gwv1.RouteGroupKind{}
	for group, kinds := range supportedRouteKindsForProtocol {
		for _, kind := range kinds {
			rgks = append(rgks, gwv1.RouteGroupKind{
				Group: (*gwv1.Group)(&group),
				Kind:  gwv1.Kind(kind),
			})
		}
	}
	return rgks
}

func validateSupportedRoutes(listeners []ir.Listener, reporter reports.GatewayReporter) []ir.Listener {
	supportedProtocolToKinds := getSupportedProtocolsRoutes()
	validListeners := []ir.Listener{}

	for _, listener := range listeners {
		supportedRouteKindsForProtocol, ok := supportedProtocolToKinds[string(listener.Protocol)]
		if !ok {
			// todo: log?
			reporter.ListenerName(string(listener.Name)).SetCondition(reports.ListenerCondition{
				Type:   gwv1.ListenerConditionAccepted,
				Status: metav1.ConditionFalse,
				Reason: gwv1.ListenerReasonUnsupportedProtocol, //TODO: add message
			})
			continue
		}

		if listener.AllowedRoutes == nil || len(listener.AllowedRoutes.Kinds) == 0 {
			// default to whatever route kinds we support on this protocol
			// TODO(Law): confirm this matches spec
			rgks := buildDefaultRouteKindsForProtocol(supportedRouteKindsForProtocol)
			reporter.ListenerName(string(listener.Name)).SetSupportedKinds(rgks)
			validListeners = append(validListeners, listener)
			continue
		}

		foundSupportedRouteKinds := []gwv1.RouteGroupKind{}
		foundInvalidRouteKinds := []gwv1.RouteGroupKind{}
		for _, rgk := range listener.AllowedRoutes.Kinds {
			if rgk.Group == nil {
				// default to Gateway API group if not set
				rgk.Group = getGroupName()
			}
			supportedRouteKinds, ok := supportedRouteKindsForProtocol[string(*rgk.Group)]
			if !ok || !slices.Contains(supportedRouteKinds, string(rgk.Kind)) {
				foundInvalidRouteKinds = append(foundInvalidRouteKinds, rgk)
				continue
			}
			foundSupportedRouteKinds = append(foundSupportedRouteKinds, rgk)
		}

		reporter.ListenerName(string(listener.Name)).SetSupportedKinds(foundSupportedRouteKinds)
		if len(foundInvalidRouteKinds) > 0 {
			reporter.ListenerName(string(listener.Name)).SetCondition(reports.ListenerCondition{
				Type:   gwv1.ListenerConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.ListenerReasonInvalidRouteKinds,
			})
		} else {
			validListeners = append(validListeners, listener)
		}
	}

	return validListeners
}

func validateListeners(gw *ir.Gateway, reporter reports.GatewayReporter) []ir.Listener {
	if len(gw.Listeners) == 0 {
		// gwReporter.Err("gateway must contain at least 1 listener")
	}

	validListeners := validateSupportedRoutes(gw.Listeners, reporter)

	portListeners := map[gwv1.PortNumber]*portProtocol{}
	for _, listener := range validListeners {
		protocol := listener.Protocol
		if protocol == gwv1.HTTPSProtocolType || protocol == gwv1.TLSProtocolType {
			protocol = NormalizedHTTPSTLSType
		}

		if existingListener, ok := portListeners[listener.Port]; ok {
			existingListener.protocol[protocol] = true
			existingListener.listeners = append(existingListener.listeners, listener)
			//TODO(Law): handle validation that hostname empty for udp/tcp
			if listener.Hostname != nil {
				existingListener.hostnames[*listener.Hostname]++
			} else {
				existingListener.hostnames[DefaultHostname]++
			}
		} else {
			var hostname gwv1.Hostname
			if listener.Hostname == nil {
				hostname = DefaultHostname
			} else {
				hostname = *listener.Hostname
			}
			pp := portProtocol{
				hostnames: map[gwv1.Hostname]int{
					hostname: 1,
				},
				protocol: map[gwv1.ProtocolType]bool{
					protocol: true,
				},
				listeners: []ir.Listener{listener},
			}
			portListeners[listener.Port] = &pp
		}
	}

	// reset valid listeners
	validListeners = []ir.Listener{}
	for _, pp := range portListeners {
		protocolConflict := false
		if len(pp.protocol) > 1 {
			protocolConflict = true
		}

		for _, listener := range pp.listeners {
			if protocolConflict {
				reporter.ListenerName(string(listener.Name)).SetCondition(reports.ListenerCondition{
					Type:    gwv1.ListenerConditionConflicted,
					Status:  metav1.ConditionTrue,
					Reason:  gwv1.ListenerReasonProtocolConflict,
					Message: "Found conflicting protocols on listeners, a single port can only contain listeners with compatible protocols",
				})

				// continue as protocolConflict will take precedence over hostname conflicts
				continue
			}

			var hostname gwv1.Hostname
			if listener.Hostname == nil {
				hostname = DefaultHostname
			} else {
				hostname = *listener.Hostname
			}
			if count := pp.hostnames[hostname]; count > 1 {
				reporter.ListenerName(string(listener.Name)).SetCondition(reports.ListenerCondition{
					Type:    gwv1.ListenerConditionConflicted,
					Status:  metav1.ConditionTrue,
					Reason:  gwv1.ListenerReasonHostnameConflict,
					Message: "Found conflicting hostnames on listeners, all listeners on a single port must have unique hostnames",
				})
			} else {
				// TODO should check this is exactly 1?
				validListeners = append(validListeners, listener)
			}
		}
	}

	if len(validListeners) == 0 {
		reporter.SetCondition(reports.GatewayCondition{
			Type:   gwv1.GatewayConditionAccepted,
			Status: metav1.ConditionFalse,
			Reason: gwv1.GatewayReasonListenersNotValid,
		})
		reporter.SetCondition(reports.GatewayCondition{
			Type:   gwv1.GatewayConditionProgrammed,
			Status: metav1.ConditionFalse,
			Reason: gwv1.GatewayReasonInvalid,
		})
	}

	return validListeners
}

func getGroupName() *gwv1.Group {
	g := gwv1.Group(gwv1.GroupName)
	return &g
}
