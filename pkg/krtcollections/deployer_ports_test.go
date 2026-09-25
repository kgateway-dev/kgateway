package krtcollections

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
