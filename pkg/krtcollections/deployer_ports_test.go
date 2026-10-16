package krtcollections

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// TestResolveDeployerPorts guards against rendering a Service port with a protocol the translator
// rejected: on a shared port the first (highest-precedence) listener wins, matching how the
// translator resolves a same-port protocol conflict.
func TestResolveDeployerPorts(t *testing.T) {
	const (
		tcp  = gwv1.TCPProtocolType
		udp  = gwv1.UDPProtocolType
		http = gwv1.HTTPProtocolType
	)

	cases := []struct {
		name         string
		entries      []listenerPortProtocol
		wantPorts    []int32
		wantUDPPorts []int32
	}{
		{
			name:         "tcp wins a shared port when listed first",
			entries:      []listenerPortProtocol{{port: 53, protocol: tcp}, {port: 53, protocol: udp}},
			wantPorts:    []int32{53},
			wantUDPPorts: nil,
		},
		{
			name:         "udp wins a shared port when listed first",
			entries:      []listenerPortProtocol{{port: 53, protocol: udp}, {port: 53, protocol: tcp}},
			wantPorts:    []int32{53},
			wantUDPPorts: []int32{53},
		},
		{
			name:         "a udp-only port renders udp",
			entries:      []listenerPortProtocol{{port: 5300, protocol: udp}},
			wantPorts:    []int32{5300},
			wantUDPPorts: []int32{5300},
		},
		{
			name:         "distinct ports keep their own protocol",
			entries:      []listenerPortProtocol{{port: 80, protocol: http}, {port: 5300, protocol: udp}},
			wantPorts:    []int32{80, 5300},
			wantUDPPorts: []int32{5300},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ports, udpPorts := resolveDeployerPorts(tc.entries)
			assert.ElementsMatch(t, tc.wantPorts, ports.UnsortedList())
			assert.ElementsMatch(t, tc.wantUDPPorts, udpPorts.UnsortedList())
		})
	}
}

// TestSortListenerSetsByPrecedence guards the order the deployer and Envoy transforms both rely on
// to resolve a same-port protocol conflict, oldest by creation timestamp then by namespace/name.
func TestSortListenerSetsByPrecedence(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(name string, created time.Time) *gwv1.ListenerSet {
		return &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{
			Namespace:         "ns",
			Name:              name,
			CreationTimestamp: metav1.NewTime(created),
		}}
	}

	// Oldest first by creation timestamp.
	sets := []*gwv1.ListenerSet{mk("newer", base.Add(time.Hour)), mk("older", base)}
	sortListenerSetsByPrecedence(sets)
	assert.Equal(t, []string{"older", "newer"}, []string{sets[0].Name, sets[1].Name})

	// Equal timestamps fall back to "{namespace}/{name}" order.
	tied := []*gwv1.ListenerSet{mk("b", base), mk("a", base)}
	sortListenerSetsByPrecedence(tied)
	assert.Equal(t, []string{"a", "b"}, []string{tied[0].Name, tied[1].Name})
}
