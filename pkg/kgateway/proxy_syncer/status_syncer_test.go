package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// TestInjectXListenerSetPorts verifies that injectXListenerSetPorts correctly
// populates the "port" field in each unstructured listener status entry by
// name-matching against the spec listeners — and explicitly nulls out the port
// for any listener whose port cannot be determined.
func TestInjectXListenerSetPorts(t *testing.T) {
	tests := []struct {
		name          string
		statusMap     map[string]interface{}
		specListeners []gwv1.ListenerEntry
		wantPorts     map[string]interface{} // name → expected port value (nil means explicit null)
	}{
		{
			name: "two listeners with distinct ports are set correctly",
			statusMap: map[string]interface{}{
				"listeners": []interface{}{
					map[string]interface{}{"name": "http"},
					map[string]interface{}{"name": "http-2"},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "http", Port: 90, Protocol: gwv1.HTTPProtocolType},
				{Name: "http-2", Port: 8091, Protocol: gwv1.HTTPProtocolType},
			},
			wantPorts: map[string]interface{}{
				"http":   int64(90),
				"http-2": int64(8091),
			},
		},
		{
			name: "listener with no port defaults to protocol default (HTTP→80)",
			statusMap: map[string]interface{}{
				"listeners": []interface{}{
					map[string]interface{}{"name": "http"},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "http", Port: 0, Protocol: gwv1.HTTPProtocolType},
			},
			wantPorts: map[string]interface{}{
				"http": int64(80),
			},
		},
		{
			name: "listener with unresolvable port is explicitly nulled out",
			statusMap: map[string]interface{}{
				"listeners": []interface{}{
					// port was previously persisted as stale value
					map[string]interface{}{"name": "tcp", "port": int64(9999)},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				// TCP with no port — DetectListenerPortNumber returns an error
				{Name: "tcp", Port: 0, Protocol: gwv1.TCPProtocolType},
			},
			wantPorts: map[string]interface{}{
				"tcp": nil,
			},
		},
		{
			name: "status entry with no matching spec listener is nulled out",
			statusMap: map[string]interface{}{
				"listeners": []interface{}{
					map[string]interface{}{"name": "orphan", "port": int64(1234)},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType},
			},
			wantPorts: map[string]interface{}{
				"orphan": nil,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			injectXListenerSetPorts(tc.statusMap, tc.specListeners)

			listeners, ok := tc.statusMap["listeners"].([]interface{})
			assert.True(t, ok, "listeners should remain a []interface{}")

			for _, raw := range listeners {
				m, ok := raw.(map[string]interface{})
				assert.True(t, ok)
				name, _ := m["name"].(string)
				wantPort, exists := tc.wantPorts[name]
				assert.True(t, exists, "unexpected listener name %q in result", name)
				assert.Equal(t, wantPort, m["port"],
					"listener %q: expected port %v, got %v", name, wantPort, m["port"])
			}
		})
	}
}
