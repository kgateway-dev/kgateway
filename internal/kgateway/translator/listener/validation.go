package listener

import (
	"fmt"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

const NormalizedHTTPSTLSType = "HTTPS/TLS"
const DefaultHostname = "*"

type portProtocol struct {
	hostnames map[gwv1.Hostname][]ir.Listener
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

func validateSupportedRoutes(listeners []ir.Listener, reporter reports.Reporter) []ir.Listener {
	supportedProtocolToKinds := getSupportedProtocolsRoutes()
	validListeners := []ir.Listener{}

	for _, listener := range listeners {
		supportedRouteKindsForProtocol, ok := supportedProtocolToKinds[string(listener.Protocol)]
		parentReporter := listener.GetParentReporter(reporter)
		if !ok {
			// todo: log?
			parentReporter.ListenerName(string(listener.Name)).SetCondition(reports.ListenerCondition{
				Type:    gwv1.ListenerConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.ListenerReasonUnsupportedProtocol,
				Message: fmt.Sprintf("Protocol %s is unsupported.", listener.Protocol),
			})
			continue
		}

		if listener.AllowedRoutes == nil || len(listener.AllowedRoutes.Kinds) == 0 {
			// default to whatever route kinds we support on this protocol
			// TODO(Law): confirm this matches spec
			rgks := buildDefaultRouteKindsForProtocol(supportedRouteKindsForProtocol)
			parentReporter.ListenerName(string(listener.Name)).SetSupportedKinds(rgks)
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

		parentReporter.ListenerName(string(listener.Name)).SetSupportedKinds(foundSupportedRouteKinds)
		if len(foundInvalidRouteKinds) > 0 {
			invalidKinds := make([]string, 0, len(foundInvalidRouteKinds))
			for _, rgk := range foundInvalidRouteKinds {
				invalidKinds = append(invalidKinds, string(rgk.Kind))
			}

			parentReporter.ListenerName(string(listener.Name)).SetCondition(reports.ListenerCondition{
				Type:    gwv1.ListenerConditionResolvedRefs,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.ListenerReasonInvalidRouteKinds,
				Message: fmt.Sprintf("Found invalid route kinds: [%s]", strings.Join(invalidKinds, ", ")),
			})
		} else {
			validListeners = append(validListeners, listener)
		}
	}

	return validListeners
}

func validateListeners(gw *ir.Gateway, reporter reports.Reporter) []ir.Listener {
	if len(gw.Listeners) == 0 {
		// gwReporter.Err("gateway must contain at least 1 listener")
	}

	validListeners := validateSupportedRoutes(gw.Listeners, reporter)

	portListeners := map[gwv1.PortNumber]*portProtocol{}
	// The listeners are already sorted based on listener precedence
	// The following loop groups listeners based on (i) Port (ii) Hostname
	// The resulting map will then be used to reject listeners that have protocol and hostname conflicts
	// Listeners on different ports don't need to be validated for a conflict so the example shown is only for a single port
	// Given a set of listeners :
	// 		- name: gateway-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: gateway-listener.com
	// 		- name: gateway-domain-name-conflict-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: domain-name-conflict-listener.com
	// 		- name: gateway-protocol-conflict-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: protocol-conflict-listener.com
	// 		- name: listenerset-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: listenerset-listener.com
	// 		- name: listenerset-domain-name-conflict-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: domain-name-conflict-listener.com
	// 		- name: listenerset-protocol-conflict-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: protocol-conflict-listener.com
	// This is the resulting map :
	//		[80] : {
	//		Protocol: {
	//			"TCP": true
	//			"UDP": true
	//		},
	//		Hostnames: {
	//			"gateway-listener.com": [gateway-listener],
	//			"domain-name-conflict-listener.com": [gateway-domain-name-conflict-listener,
	// 										listenerset-domain-name-conflict-listener]
	//			"protocol-conflict-listener.com": [gateway-protocol-conflict-listener, listenerset-protocol-conflict-listener]
	//			"listenerset-1-listener.com": [listenerset-listener]
	//		},
	//      Listeners: [gateway-listener, gateway-domain-name-conflict-listener,gateway-protocol-conflict-listener,
	// 					listenerset-listener, listenerset-domain-name-conflict-listener, listenerset-protocol-conflict-listener]
	//	}
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
				existingListener.hostnames[*listener.Hostname] = append(existingListener.hostnames[*listener.Hostname], listener)
			} else {
				existingListener.hostnames[DefaultHostname] = append(existingListener.hostnames[DefaultHostname], listener)
			}
		} else {
			var hostname gwv1.Hostname
			if listener.Hostname == nil {
				hostname = DefaultHostname
			} else {
				hostname = *listener.Hostname
			}
			pp := portProtocol{
				hostnames: map[gwv1.Hostname][]ir.Listener{
					hostname: []ir.Listener{listener},
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

	// This loop validates listeners based on any port / protocol / hostname conflict
	// Based on the example, the list of validListeners will as follows
	// 		- name: gateway-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: gateway-listener.com
	// 		- name: gateway-domain-name-conflict-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: domain-name-conflict-listener.com
	// 		- name: gateway-protocol-conflict-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: protocol-conflict-listener.com
	// 		- name: listenerset-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: listenerset-listener.com
	// The following listeners are rejected :
	// 		- name: listenerset-domain-name-conflict-listener	<----- conflicts with gateway-domain-name-conflict-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: domain-name-conflict-listener.com
	// 		- name: listenerset-protocol-conflict-listener		<----- conflicts with gateway-protocol-conflict-listener
	// 		  port: 80
	// 		  protocol: HTTP
	// 		  hostname: protocol-conflict-listener.com

	for _, pp := range portListeners {
		for _, listener := range pp.listeners {
			parentReporter := listener.GetParentReporter(reporter)
			if validateProtocolConflict(*pp, listener) {
				// If a listener does not have a protocol conflict with one listener,
				// it could still have a hostname conflict with another listener
				if validateHostNameConflict(*pp, listener) {
					validListeners = append(validListeners, listener)
				} else {
					rejectConflictedListener(parentReporter, listener, gwv1.ListenerReasonHostnameConflict, ListenerMessageHostnameConflict)
				}
			} else {
				rejectConflictedListener(parentReporter, listener, gwv1.ListenerReasonProtocolConflict, ListenerMessageProtocolConflict)
			}
		}
	}

	// Add the final conditions on the Gateway
	if gw.Obj.Spec.AllowedListeners == nil {
		reporter.Gateway(gw.Obj).SetCondition(reports.GatewayCondition{
			Type:   GatewayConditionAttachedListenerSets,
			Status: metav1.ConditionUnknown,
			Reason: GatewayReasonListenerSetsNotAllowed,
		})
		return validListeners
	}

	if len(validListeners) == 0 {
		reporter.Gateway(gw.Obj).SetCondition(reports.GatewayCondition{
			Type:   gwv1.GatewayConditionAccepted,
			Status: metav1.ConditionFalse,
			Reason: gwv1.GatewayReasonListenersNotValid,
		})
		reporter.Gateway(gw.Obj).SetCondition(reports.GatewayCondition{
			Type:   gwv1.GatewayConditionProgrammed,
			Status: metav1.ConditionFalse,
			Reason: gwv1.GatewayReasonInvalid,
		})
		return validListeners
	}

	listenerSetListenerExists := false
	for _, listener := range validListeners {
		if _, ok := listener.Parent.(*gwxv1a1.XListenerSet); ok {
			listenerSetListenerExists = true
			break
		}
	}

	if listenerSetListenerExists {
		reporter.Gateway(gw.Obj).SetCondition(reports.GatewayCondition{
			Type:   GatewayConditionAttachedListenerSets,
			Status: metav1.ConditionTrue,
			Reason: GatewayReasonListenerSetsAttached,
		})
	} else {
		reporter.Gateway(gw.Obj).SetCondition(reports.GatewayCondition{
			Type:   GatewayConditionAttachedListenerSets,
			Status: metav1.ConditionFalse,
			Reason: gwv1.GatewayReasonNoResources,
		})
	}
	return validListeners
}

func validateGateway(consolidatedGateway *ir.Gateway, reporter reports.Reporter) []ir.Listener {
	rejectDeniedListenerSets(consolidatedGateway, reporter)
	validatedListeners := validateListeners(consolidatedGateway, reporter)
	return validatedListeners
}

func validateHostNameConflict(portProtocol portProtocol, listener ir.Listener) bool {
	// An example of the portProtocol passed to this method :
	//	{
	//		Protocol: {
	//			"TCP": true
	//			"UDP": true
	//		},
	//		Hostnames: {
	//			"gateway-listener.com": {gateway-listener},
	//			"domain-name-conflict-listener.com": {gateway-domain-name-conflict-listener, listenerset-domain-name-conflict-listener}
	//			"protocol-conflict-listener.com": {gateway-protocol-conflict-listener, listenerset-protocol-conflict-listener}
	//			"listenerset-1-listener.com": {listenerset-listener}
	//		},
	//      Listeners: [gateway-listener, gateway-domain-name-conflict-listener,gateway-protocol-conflict-listener,
	// 					listenerset-listener, listenerset-domain-name-conflict-listener, listenerset-protocol-conflict-listener]
	//	}
	var hostname gwv1.Hostname
	if listener.Hostname == nil {
		hostname = DefaultHostname
	} else {
		hostname = *listener.Hostname
	}
	if count := len(portProtocol.hostnames[hostname]); count > 1 {
		// The first listener in the list of sorted listeners is always accepted - based on listener precedence
		// The listeners with protocol conflicts have already been removed at this point
		// Accept only the first listener with a unique hostname
		// Based on the example, the following listeners will be accepted :
		// "gateway-listener.com": {gateway-listener},
		// "domain-name-conflict-listener.com": {gateway-domain-name-conflict-listener}
		// "protocol-conflict-listener.com": {gateway-protocol-conflict-listener}
		// "listenerset-1-listener.com": {listenerset-listener}
		if listener.Equals(portProtocol.hostnames[hostname][0]) {
			logger.Info("accepted listener with hostname conflict as per listener precedence", "name", listener.Name, "parent", listener.Parent.GetName())
			return true
		}
		// Based on the example, the following listeners will be rejected :
		// "domain-name-conflict-listener.com": {listenerset-domain-name-conflict-listener}
		// "protocol-conflict-listener.com": {listenerset-protocol-conflict-listener}
		logger.Error("rejected listener with hostname conflict as per listener precedence", "name", listener.Name, "parent", listener.Parent.GetName())
		return false
	} else {
		return true
	}
}

func validateProtocolConflict(portProtocol portProtocol, listener ir.Listener) bool {
	// An example of the portProtocol passed to this method :
	//	{
	//		Protocol: {
	//			"TCP": true
	//			"UDP": true
	//		},
	//		Hostnames: {
	//			"gateway-listener.com": {gateway-listener},
	//			"domain-name-conflict-listener.com": {gateway-domain-name-conflict-listener, listenerset-domain-name-conflict-listener}
	//			"protocol-conflict-listener.com": {gateway-protocol-conflict-listener, listenerset-protocol-conflict-listener}
	//			"listenerset-1-listener.com": {listenerset-listener}
	//		},
	//      Listeners: [gateway-listener, gateway-domain-name-conflict-listener,gateway-protocol-conflict-listener,
	// 					listenerset-listener, listenerset-domain-name-conflict-listener, listenerset-protocol-conflict-listener]
	//	}
	protocolConflict := false
	if len(portProtocol.protocol) > 1 {
		protocolConflict = true
	}
	// In this example, protocolConflict = true
	if protocolConflict {
		// The first listener in the list of sorted listeners is always accepted - based on listener precedence
		// Accept all listeners with the same protocol as the first listener (gateway-listener). If not, only the `gateway-listener` will be accepted and all other TCP listeners will be rejected.
		// This can lead to a situation where one UDP listener on the same port takes down all but the first TCP listener
		// Listeners [gateway-domain-name-conflict-listener, listenerset-listener, listenerset-domain-name-conflict-listener] are accepted - hostname validation will happen later
		if listener.Protocol == portProtocol.listeners[0].Protocol {
			logger.Info("accepted listener with protocol conflict as per listener precedence", "name", listener.Name, "parent", listener.Parent.GetName())
			return true
		}
		// Listeners with protocols that do not match the first listener are rejected
		// Listeners [listenerset-protocol-conflict-listener] are rejected
		logger.Error("rejected listener with protocol conflict as per listener precedence", "name", listener.Name, "parent", listener.Parent.GetName())
		return false
	}
	return true
}

func rejectDeniedListenerSets(consolidatedGateway *ir.Gateway, reporter reports.Reporter) {
	for _, ls := range consolidatedGateway.DeniedListenerSets {
		acceptedCond := reports.GatewayCondition{
			Type:   gwv1.GatewayConditionType(gwxv1a1.ListenerSetConditionAccepted),
			Status: metav1.ConditionFalse,
			Reason: gwv1.GatewayConditionReason(gwxv1a1.ListenerSetReasonNotAllowed),
		}
		if ls.Err != nil {
			acceptedCond.Message = ls.Err.Error()
		}
		reporter.ListenerSet(ls.Obj).SetCondition(acceptedCond)
		programmedCond := reports.GatewayCondition{
			Type:   gwv1.GatewayConditionType(gwxv1a1.ListenerSetConditionProgrammed),
			Status: metav1.ConditionFalse,
			Reason: gwv1.GatewayConditionReason(gwxv1a1.ListenerSetReasonNotAllowed),
		}
		if ls.Err != nil {
			programmedCond.Message = ls.Err.Error()
		}
		reporter.ListenerSet(ls.Obj).SetCondition(programmedCond)
	}
}

func getGroupName() *gwv1.Group {
	g := gwv1.Group(gwv1.GroupName)
	return &g
}

func rejectConflictedListener(parentReporter reports.GatewayReporter, listener ir.Listener, reason gwv1.ListenerConditionReason, message string) {
	parentReporter.ListenerName(string(listener.Name)).SetCondition(reports.ListenerCondition{
		Type:    gwv1.ListenerConditionConflicted,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	parentReporter.ListenerName(string(listener.Name)).SetCondition(reports.ListenerCondition{
		Type:    gwv1.ListenerConditionAccepted,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	parentReporter.ListenerName(string(listener.Name)).SetCondition(reports.ListenerCondition{
		Type:    gwv1.ListenerConditionProgrammed,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	// Set the accepted and programmed condition now since the right reason is needed.
	// If the gateway is eventually rejected, the condition will be overwritten
	// TODO: Link to gep
	parentReporter.SetCondition(reports.GatewayCondition{
		Type:   gwv1.GatewayConditionAccepted,
		Status: metav1.ConditionTrue,
		Reason: gwv1.GatewayConditionReason(gwxv1a1.ListenerSetReasonListenersNotValid),
	})
	parentReporter.SetCondition(reports.GatewayCondition{
		Type:   gwv1.GatewayConditionProgrammed,
		Status: metav1.ConditionTrue,
		Reason: gwv1.GatewayConditionReason(gwxv1a1.ListenerSetReasonListenersNotValid),
	})
}
