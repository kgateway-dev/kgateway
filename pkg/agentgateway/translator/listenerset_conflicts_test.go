package translator

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayx "sigs.k8s.io/gateway-api/apisx/v1alpha1"
)

func hostnamePtr(s string) *gwv1.Hostname {
	h := gwv1.Hostname(s)
	return &h
}

const (
	defaultGatewayName      = "gw"
	defaultGatewayNamespace = "default"
)

func parentRefToGateway() gatewayx.ParentGatewayReference {
	ns := gatewayx.Namespace(defaultGatewayNamespace)
	return gatewayx.ParentGatewayReference{
		Name:      gatewayx.ObjectName(defaultGatewayName),
		Namespace: &ns,
	}
}

func TestDetectConflictsProtocolConflict(t *testing.T) {
	cands := []candidateListener{
		{
			key:          "Gateway/default/gw.http",
			kind:         "Gateway",
			namespace:    "default",
			name:         "gw",
			listenerName: "http",
			port:         80,
			protocol:     gwv1.HTTPProtocolType,
			hostname:     hostnamePtr("example.com"),
		},
		{
			key:          "XListenerSet/ns/ls.tcp",
			kind:         "XListenerSet",
			namespace:    "ns",
			name:         "ls",
			listenerName: "tcp",
			port:         80,
			protocol:     gwv1.TCPProtocolType,
			hostname:     hostnamePtr("example.com"),
		},
	}

	conflicts := detectConflicts(cands)
	ci, ok := conflicts["XListenerSet/ns/ls.tcp"]
	if !ok {
		t.Fatalf("expected protocol conflict for XListenerSet/ns/ls.tcp")
	}
	if ci.conflictedReason != string(gatewayx.ListenerEntryReasonProtocolConflict) {
		t.Fatalf("expected reason %q, got %q", gatewayx.ListenerEntryReasonProtocolConflict, ci.conflictedReason)
	}
	if ci.winner.key != "Gateway/default/gw.http" {
		t.Fatalf("expected winner Gateway/default/gw.http, got %q", ci.winner.key)
	}
}

func TestDetectConflictsListenerConflictForProtocolAndHostname(t *testing.T) {
	cands := []candidateListener{
		{
			key:          "Gateway/default/gw.http",
			kind:         "Gateway",
			namespace:    "default",
			name:         "gw",
			listenerName: "http",
			port:         80,
			protocol:     gwv1.HTTPProtocolType,
			hostname:     hostnamePtr("example.com"),
		},
		{
			key:          "XListenerSet/ns/ls.https",
			kind:         "XListenerSet",
			namespace:    "ns",
			name:         "ls",
			listenerName: "https",
			port:         80,
			protocol:     gwv1.HTTPSProtocolType,
			hostname:     hostnamePtr("example.com"),
		},
	}

	conflicts := detectConflicts(cands)
	ci, ok := conflicts["XListenerSet/ns/ls.https"]
	if !ok {
		t.Fatalf("expected listener conflict for XListenerSet/ns/ls.https")
	}
	if ci.conflictedReason != string(gatewayx.ListenerEntryReasonListenerConflict) {
		t.Fatalf("expected reason %q, got %q", gatewayx.ListenerEntryReasonListenerConflict, ci.conflictedReason)
	}
}

func TestDetectConflictsHostnameConflict(t *testing.T) {
	cands := []candidateListener{
		{
			key:          "Gateway/default/gw.http",
			kind:         "Gateway",
			namespace:    "default",
			name:         "gw",
			listenerName: "http",
			port:         80,
			protocol:     gwv1.HTTPProtocolType,
			hostname:     hostnamePtr("example.com"),
		},
		{
			key:          "XListenerSet/ns/ls.http",
			kind:         "XListenerSet",
			namespace:    "ns",
			name:         "ls",
			listenerName: "http",
			port:         80,
			protocol:     gwv1.HTTPProtocolType,
			hostname:     hostnamePtr("example.com"),
		},
	}

	conflicts := detectConflicts(cands)
	ci, ok := conflicts["XListenerSet/ns/ls.http"]
	if !ok {
		t.Fatalf("expected hostname conflict for XListenerSet/ns/ls.http")
	}
	if ci.conflictedReason != string(gatewayx.ListenerEntryReasonHostnameConflict) {
		t.Fatalf("expected reason %q, got %q", gatewayx.ListenerEntryReasonHostnameConflict, ci.conflictedReason)
	}
	if ci.winner.key != "Gateway/default/gw.http" {
		t.Fatalf("expected winner Gateway/default/gw.http, got %q", ci.winner.key)
	}
}

func TestDetectConflictsWildcardExactNoConflict(t *testing.T) {
	cands := []candidateListener{
		{
			key:          "Gateway/default/gw.foo",
			kind:         "Gateway",
			namespace:    "default",
			name:         "gw",
			listenerName: "foo",
			port:         80,
			protocol:     gwv1.HTTPProtocolType,
			hostname:     hostnamePtr("foo.example.com"),
		},
		{
			key:          "XListenerSet/ns/ls.wildcard",
			kind:         "XListenerSet",
			namespace:    "ns",
			name:         "ls",
			listenerName: "wildcard",
			port:         80,
			protocol:     gwv1.HTTPProtocolType,
			hostname:     hostnamePtr("*.example.com"),
		},
	}

	conflicts := detectConflicts(cands)
	if _, ok := conflicts["XListenerSet/ns/ls.wildcard"]; ok {
		t.Fatalf("expected wildcard/exact hostname not to conflict")
	}
}

func TestDetectConflictsTCPPortOnly(t *testing.T) {
	cands := []candidateListener{
		{
			key:          "Gateway/default/gw.tcp-1",
			kind:         "Gateway",
			namespace:    "default",
			name:         "gw",
			listenerName: "tcp-1",
			port:         80,
			protocol:     gwv1.TCPProtocolType,
			hostname:     hostnamePtr("a.example.com"),
		},
		{
			key:          "XListenerSet/ns/ls.tcp-2",
			kind:         "XListenerSet",
			namespace:    "ns",
			name:         "ls",
			listenerName: "tcp-2",
			port:         80,
			protocol:     gwv1.TCPProtocolType,
			hostname:     hostnamePtr("b.example.com"),
		},
	}

	conflicts := detectConflicts(cands)
	ci, ok := conflicts["XListenerSet/ns/ls.tcp-2"]
	if !ok {
		t.Fatalf("expected TCP listeners on same port to conflict")
	}
	if ci.conflictedReason != string(gatewayx.ListenerEntryReasonListenerConflict) {
		t.Fatalf("expected reason %q, got %q", gatewayx.ListenerEntryReasonListenerConflict, ci.conflictedReason)
	}
}

func TestGatewayWinsOverListenerSetSameTuple(t *testing.T) {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Hostname: hostnamePtr("example.com"),
				},
			},
		},
	}

	ls := &gatewayx.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ls",
			Namespace: "default",
		},
		Spec: gatewayx.ListenerSetSpec{
			ParentRef: parentRefToGateway(),
			Listeners: []gatewayx.ListenerEntry{
				{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Hostname: hostnamePtr("example.com"),
				},
			},
		},
	}

	cands := buildCandidatesForGateway(gw, []*gatewayx.XListenerSet{ls}, ls, nil)
	conflicts := detectConflicts(cands)

	key := uniqueListenerKey(kindListenerSet, ls.Namespace, ls.Name, "http")
	ci, ok := conflicts[key]
	if !ok {
		t.Fatalf("expected ListenerSet listener to be conflicted")
	}
	if ci.conflictedReason != string(gatewayx.ListenerEntryReasonHostnameConflict) {
		t.Fatalf("expected reason %q, got %q", gatewayx.ListenerEntryReasonHostnameConflict, ci.conflictedReason)
	}
	if ci.winner.key != uniqueListenerKey(kindGateway, "default", "gw", "http") {
		t.Fatalf("expected Gateway to win, got %q", ci.winner.key)
	}
}

func TestDetectConflictsHTTPSAndTLSNormalization(t *testing.T) {
	cands := []candidateListener{
		{
			key:          "Gateway/default/gw.https",
			kind:         "Gateway",
			namespace:    "default",
			name:         "gw",
			listenerName: "https",
			port:         443,
			protocol:     gwv1.HTTPSProtocolType,
			hostname:     hostnamePtr("example.com"),
		},
		{
			key:          "XListenerSet/ns/ls.tls",
			kind:         "XListenerSet",
			namespace:    "ns",
			name:         "ls",
			listenerName: "tls",
			port:         443,
			protocol:     gwv1.TLSProtocolType,
			hostname:     hostnamePtr("example.com"),
		},
	}

	conflicts := detectConflicts(cands)
	ci, ok := conflicts["XListenerSet/ns/ls.tls"]
	if !ok {
		t.Fatalf("expected conflict for XListenerSet/ns/ls.tls")
	}
	if ci.conflictedReason != string(gatewayx.ListenerEntryReasonHostnameConflict) {
		t.Fatalf("expected reason %q, got %q", gatewayx.ListenerEntryReasonHostnameConflict, ci.conflictedReason)
	}
}

func TestAllowListenersFiltering(t *testing.T) {
	from := gwv1.NamespacesFromSame
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
				},
			},
			AllowedListeners: &gwv1.AllowedListeners{
				Namespaces: &gwv1.ListenerNamespaces{From: &from},
			},
		},
	}

	ls := &gatewayx.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ls",
			Namespace: "other",
		},
		Spec: gatewayx.ListenerSetSpec{
			ParentRef: parentRefToGateway(),
			Listeners: []gatewayx.ListenerEntry{
				{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
				},
			},
		},
	}

	lookupNamespace := func(name string) *corev1.Namespace {
		return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}
	cands := buildCandidatesForGateway(gw, []*gatewayx.XListenerSet{ls}, ls, lookupNamespace)
	for _, c := range cands {
		if c.kind == kindListenerSet {
			t.Fatalf("expected ListenerSet from disallowed namespace to be excluded")
		}
	}
}

func TestBuildCandidatesForGatewayListenerSetOrder(t *testing.T) {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
				},
			},
		},
	}

	lsOld := &gatewayx.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "a-listenerset",
			Namespace:         "ns",
			CreationTimestamp: metav1.NewTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
		Spec: gatewayx.ListenerSetSpec{
			ParentRef: parentRefToGateway(),
			Listeners: []gatewayx.ListenerEntry{
				{
					Name:     "ls",
					Protocol: gwv1.HTTPProtocolType,
					Port:     90,
				},
			},
		},
	}

	lsNew := &gatewayx.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "b-listenerset",
			Namespace:         "ns",
			CreationTimestamp: metav1.NewTime(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
		},
		Spec: gatewayx.ListenerSetSpec{
			ParentRef: parentRefToGateway(),
			Listeners: []gatewayx.ListenerEntry{
				{
					Name:     "ls",
					Protocol: gwv1.HTTPProtocolType,
					Port:     90,
				},
			},
		},
	}

	cands := buildCandidatesForGateway(gw, []*gatewayx.XListenerSet{lsNew, lsOld}, lsNew, nil)
	conflicts := detectConflicts(cands)

	oldKey := uniqueListenerKey("XListenerSet", lsOld.Namespace, lsOld.Name, "ls")
	newKey := uniqueListenerKey("XListenerSet", lsNew.Namespace, lsNew.Name, "ls")

	if _, ok := conflicts[oldKey]; ok {
		t.Fatalf("expected older ListenerSet to win, but it was marked conflicted")
	}
	if _, ok := conflicts[newKey]; !ok {
		t.Fatalf("expected newer ListenerSet to be conflicted")
	}
}

func TestApplyConflictToListenerEntryConditionsSetsPortUnavailable(t *testing.T) {
	ci := conflictInfo{
		conflictedReason: string(gatewayx.ListenerEntryReasonProtocolConflict),
		winner: conflictWinner{
			key:       "Gateway/default/gw.http",
			kind:      "Gateway",
			namespace: "default",
			name:      "gw",
		},
		message: "protocol \"TCP\" conflicts with \"HTTP\" on port 80",
	}

	conds := applyConflictToListenerEntryConditions(1, nil, ci, "default")
	find := func(t string) *metav1.Condition {
		for i := range conds {
			if conds[i].Type == t {
				return &conds[i]
			}
		}
		return nil
	}

	conflicted := find(string(gatewayx.ListenerEntryConditionConflicted))
	if conflicted == nil || conflicted.Status != metav1.ConditionTrue || conflicted.Reason != string(gatewayx.ListenerEntryReasonProtocolConflict) {
		t.Fatalf("expected Conflicted=True with ProtocolConflict, got %#v", conflicted)
	}

	accepted := find(string(gatewayx.ListenerEntryConditionAccepted))
	if accepted == nil || accepted.Status != metav1.ConditionFalse || accepted.Reason != string(gatewayx.ListenerEntryReasonPortUnavailable) {
		t.Fatalf("expected Accepted=False with PortUnavailable, got %#v", accepted)
	}

	programmed := find(string(gatewayx.ListenerEntryConditionProgrammed))
	if programmed == nil || programmed.Status != metav1.ConditionFalse || programmed.Reason != string(gatewayx.ListenerEntryReasonPortUnavailable) {
		t.Fatalf("expected Programmed=False with PortUnavailable, got %#v", programmed)
	}
}

func TestApplyConflictToListenerEntryConditionsCrossNamespaceWinnerHint(t *testing.T) {
	ci := conflictInfo{
		conflictedReason: string(gatewayx.ListenerEntryReasonHostnameConflict),
		winner: conflictWinner{
			key:       "XListenerSet/other/ls.http",
			kind:      kindListenerSet,
			namespace: "other",
			name:      "ls",
		},
		message: "hostname \"example.com\" conflicts with another listener on port 80",
	}

	conds := applyConflictToListenerEntryConditions(1, nil, ci, "default")
	var conflicted *metav1.Condition
	for i := range conds {
		if conds[i].Type == string(gatewayx.ListenerEntryConditionConflicted) {
			conflicted = &conds[i]
			break
		}
	}
	if conflicted == nil {
		t.Fatalf("expected conflicted condition")
	}
	if !strings.Contains(conflicted.Message, "winner: another ListenerSet with higher precedence") {
		t.Fatalf("expected redacted winner hint, got %q", conflicted.Message)
	}
	if strings.Contains(conflicted.Message, "ls") {
		t.Fatalf("expected winner name to be redacted, got %q", conflicted.Message)
	}
}

func TestIndexDriftWithSkippedListenerPort(t *testing.T) {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Hostname: hostnamePtr("example.com"),
				},
			},
		},
	}

	ls := &gatewayx.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ls",
			Namespace: "default",
		},
		Spec: gatewayx.ListenerSetSpec{
			ParentRef: parentRefToGateway(),
			Listeners: []gatewayx.ListenerEntry{
				{
					Name:     "bad",
					Protocol: gwv1.TCPProtocolType,
					Port:     0,
				},
				{
					Name:     "good",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Hostname: hostnamePtr("example.com"),
				},
			},
		},
	}

	cands := buildCandidatesForGateway(gw, []*gatewayx.XListenerSet{ls}, ls, nil)
	conflicts := detectConflicts(cands)

	status := gatewayx.ListenerSetStatus{
		Listeners: []gatewayx.ListenerEntryStatus{
			{Name: "bad"},
			{Name: "good"},
		},
	}

	for _, c := range cands {
		if !c.isCurrentLS {
			continue
		}
		ci, ok := conflicts[c.key]
		if !ok {
			continue
		}
		status.Listeners[c.currentIdx].Conditions = applyConflictToListenerEntryConditions(
			1,
			status.Listeners[c.currentIdx].Conditions,
			ci,
			ls.Namespace,
		)
	}

	if len(status.Listeners[0].Conditions) != 0 {
		t.Fatalf("expected invalid listener entry to remain unchanged")
	}
	if len(status.Listeners[1].Conditions) == 0 {
		t.Fatalf("expected valid listener entry to be updated with conflict conditions")
	}
}
