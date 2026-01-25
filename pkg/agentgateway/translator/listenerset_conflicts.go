package translator

import (
	"fmt"
	"sort"

	"istio.io/istio/pilot/pkg/model/kstatus"
	"istio.io/istio/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayx "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	normalizedHTTPSTLS gwv1.ProtocolType = "HTTPS/TLS"

	kindGateway     = "Gateway"
	kindListenerSet = "XListenerSet"
)

type candidateListener struct {
	key          string
	kind         string
	namespace    string
	name         string
	listenerName string
	port         gwv1.PortNumber
	protocol     gwv1.ProtocolType
	hostname     *gwv1.Hostname

	// true if this candidate belongs to the ListenerSet currently being reconciled
	isCurrentLS bool
	// listener index within the current ListenerSet (only valid if isCurrentLS is true)
	currentIdx int
}

type conflictInfo struct {
	conflictedReason string
	winner           conflictWinner
	message          string
}

type conflictWinner struct {
	key       string
	kind      string
	namespace string
	name      string
}

func defaultHostname(h *gwv1.Hostname) gwv1.Hostname {
	if h == nil || *h == "" {
		return gwv1.Hostname("*")
	}
	return *h
}

func normalizeProtocol(p gwv1.ProtocolType) gwv1.ProtocolType {
	if p == gwv1.HTTPSProtocolType || p == gwv1.TLSProtocolType {
		return normalizedHTTPSTLS
	}
	return p
}

func protocolSupportsHostname(p gwv1.ProtocolType) bool {
	return p != gwv1.TCPProtocolType && p != gwv1.ProtocolType("UDP")
}

func uniqueListenerKey(kind, ns, name, listenerName string) string {
	return fmt.Sprintf("%s/%s/%s.%s", kind, ns, name, listenerName)
}

// detectConflicts detects protocol and hostname conflicts across a precedence-ordered list of candidates.
func detectConflicts(cands []candidateListener) map[string]conflictInfo {
	type portState struct {
		winnerProto gwv1.ProtocolType
		winner      conflictWinner
		hostnames   map[gwv1.Hostname]conflictWinner
	}

	state := map[gwv1.PortNumber]*portState{}
	conflicts := map[string]conflictInfo{}

	for _, c := range cands {
		port := c.port
		proto := normalizeProtocol(c.protocol)
		supportsHostname := protocolSupportsHostname(proto)
		hostname := defaultHostname(c.hostname)

		ps, ok := state[port]
		if !ok {
			winner := conflictWinner{
				key:       c.key,
				kind:      c.kind,
				namespace: c.namespace,
				name:      c.name,
			}
			state[port] = &portState{
				winnerProto: proto,
				winner:      winner,
				hostnames:   map[gwv1.Hostname]conflictWinner{},
			}
			if supportsHostname {
				state[port].hostnames[hostname] = winner
			}
			continue
		}

		if proto != ps.winnerProto {
			conflictedReason := string(gatewayx.ListenerEntryReasonProtocolConflict)
			message := fmt.Sprintf("protocol %q conflicts with %q on port %d", proto, ps.winnerProto, port)
			if supportsHostname {
				if _, exists := ps.hostnames[hostname]; exists {
					conflictedReason = string(gatewayx.ListenerEntryReasonListenerConflict)
					message = fmt.Sprintf("listener conflicts with multiple listeners on port %d", port)
				}
			}
			conflicts[c.key] = conflictInfo{
				conflictedReason: conflictedReason,
				winner:           ps.winner,
				message:          message,
			}
			continue
		}

		if !supportsHostname {
			conflicts[c.key] = conflictInfo{
				conflictedReason: string(gatewayx.ListenerEntryReasonListenerConflict),
				winner:           ps.winner,
				message:          fmt.Sprintf("port %d conflicts with another listener", port),
			}
			continue
		}

		if win, exists := ps.hostnames[hostname]; exists {
			conflicts[c.key] = conflictInfo{
				conflictedReason: string(gatewayx.ListenerEntryReasonHostnameConflict),
				winner:           win,
				message:          fmt.Sprintf("hostname %q conflicts with another listener on port %d", hostname, port),
			}
			continue
		}

		ps.hostnames[hostname] = conflictWinner{
			key:       c.key,
			kind:      c.kind,
			namespace: c.namespace,
			name:      c.name,
		}
	}

	return conflicts
}

func applyConflictToListenerEntryConditions(
	generation int64,
	existing []metav1.Condition,
	ci conflictInfo,
	currentNamespace string,
) []metav1.Condition {
	winnerHint := ""
	switch ci.winner.kind {
	case kindGateway:
		winnerHint = fmt.Sprintf(" (winner: Gateway %s/%s)", ci.winner.namespace, ci.winner.name)
	case kindListenerSet:
		if ci.winner.namespace == currentNamespace {
			winnerHint = fmt.Sprintf(" (winner: ListenerSet %s)", ci.winner.name)
		} else {
			winnerHint = " (winner: another ListenerSet with higher precedence)"
		}
	}

	conflictedMsg := fmt.Sprintf("%s%s", ci.message, winnerHint)

	conds := map[string]*Condition{
		string(gatewayx.ListenerEntryConditionConflicted): {
			Status:  kstatus.StatusTrue,
			Reason:  ci.conflictedReason,
			Message: conflictedMsg,
		},
		string(gatewayx.ListenerEntryConditionAccepted): {
			Error: &ConfigError{Reason: string(gatewayx.ListenerEntryReasonPortUnavailable), Message: conflictedMsg},
		},
		string(gatewayx.ListenerEntryConditionProgrammed): {
			Error: &ConfigError{Reason: string(gatewayx.ListenerEntryReasonPortUnavailable), Message: conflictedMsg},
		},
	}

	return SetConditions(generation, existing, conds)
}

// Build the precedence-ordered candidate list:
// 1) Gateway listeners (spec order)
// 2) ListenerSets sorted by creationTimestamp, then namespace/name (spec order within each ListenerSet)
func buildCandidatesForGateway(
	parentGw *gwv1.Gateway,
	allLS []*gatewayx.XListenerSet,
	current *gatewayx.XListenerSet,
	lookupNamespace func(string) *corev1.Namespace,
) []candidateListener {
	cands := []candidateListener{}

	for _, l := range parentGw.Spec.Listeners {
		cands = append(cands, candidateListener{
			key:          uniqueListenerKey(kindGateway, parentGw.Namespace, parentGw.Name, string(l.Name)),
			kind:         kindGateway,
			namespace:    parentGw.Namespace,
			name:         parentGw.Name,
			listenerName: string(l.Name),
			port:         l.Port,
			protocol:     l.Protocol,
			hostname:     l.Hostname,
		})
	}

	attached := make([]*gatewayx.XListenerSet, 0, len(allLS))
	for _, ls := range allLS {
		p := ls.Spec.ParentRef
		if NormalizeReference(p.Group, p.Kind, wellknown.GatewayGVK) != wellknown.GatewayGVK {
			continue
		}
		pns := string(ptr.OrDefault(p.Namespace, gatewayx.Namespace(ls.Namespace)))
		if pns != parentGw.Namespace || string(p.Name) != parentGw.Name {
			continue
		}
		if lookupNamespace != nil && !NamespaceAcceptedByAllowListeners(ls.Namespace, parentGw, lookupNamespace) {
			continue
		}
		attached = append(attached, ls)
	}

	sort.SliceStable(attached, func(i, j int) bool {
		ti := attached[i].CreationTimestamp.Time
		tj := attached[j].CreationTimestamp.Time
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		if attached[i].Namespace != attached[j].Namespace {
			return attached[i].Namespace < attached[j].Namespace
		}
		return attached[i].Name < attached[j].Name
	})

	for _, ls := range attached {
		for idx, le := range ls.Spec.Listeners {
			port, err := kubeutils.DetectListenerPortNumber(le.Protocol, le.Port)
			if err != nil {
				continue
			}
			l := gwv1.Listener(le)

			cands = append(cands, candidateListener{
				key:          uniqueListenerKey(kindListenerSet, ls.Namespace, ls.Name, string(l.Name)),
				kind:         kindListenerSet,
				namespace:    ls.Namespace,
				name:         ls.Name,
				listenerName: string(l.Name),
				port:         port,
				protocol:     l.Protocol,
				hostname:     l.Hostname,
				isCurrentLS:  ls.Namespace == current.Namespace && ls.Name == current.Name,
				currentIdx:   idx,
			})
		}
	}

	return cands
}
