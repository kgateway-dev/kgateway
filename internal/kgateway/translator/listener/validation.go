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
	for _, listener := range validListeners {
		protocol := listener.Protocol
		if protocol == gwv1.HTTPSProtocolType || protocol == gwv1.TLSProtocolType {
			protocol = NormalizedHTTPSTLSType
		}

		// TODO: Keep the first listener in case of a conflict
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

		for idx, listener := range pp.listeners {
			parentReporter := listener.GetParentReporter(reporter)

			// There should be no need to check for port / protocol / hostname conflicts on gateway listeners
			// as that is handled by kube validation
			if protocolConflict {
				// Accept the first conflicted listener - they have already been sorted by listener precedence
				// TODO(davidjumani): Link to GEP when https://github.com/kubernetes-sigs/gateway-api/pull/3978 merges
				if idx == 0 {
					logger.Info("accepted listener with protocol conflict as per listener precedence", "name", listener.Name, "parent", listener.Parent.GetName())
					validListeners = append(validListeners, listener)
					continue
				}

				logger.Error("rejected listener with protocol conflict as per listener precedence", "name", listener.Name, "parent", listener.Parent.GetName())
				rejectConflictedListener(parentReporter, listener, gwv1.ListenerReasonProtocolConflict, ListenerMessageProtocolConflict)

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
				// Accept the first conflicted listener - they have already been sorted by listener precedence
				// TODO(davidjumani): Link to GEP when https://github.com/kubernetes-sigs/gateway-api/pull/3978 merges
				if idx == 0 {
					logger.Info("accepted listener with hostname conflict as per listener precedence", "name", listener.Name, "parent", listener.Parent.GetName())
					validListeners = append(validListeners, listener)
					continue
				}

				logger.Error("rejected listener with hostname conflict as per listener precedence", "name", listener.Name, "parent", listener.Parent.GetName())
				rejectConflictedListener(parentReporter, listener, gwv1.ListenerReasonHostnameConflict, ListenerMessageHostnameConflict)
			} else {
				// TODO should check this is exactly 1?
				validListeners = append(validListeners, listener)
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
