package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestInjectListenerPorts(t *testing.T) {
	tests := []struct {
		name          string
		statusMap     map[string]any
		specListeners []gwv1.ListenerEntry
		wantPorts     map[string]int64 // listener name -> expected port
	}{
		{
			name: "spec explicit port is injected as-is",
			statusMap: map[string]any{
				"listeners": []any{
					map[string]any{"name": "http"},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 8080},
			},
			wantPorts: map[string]int64{"http": 8080},
		},
		{
			name: "HTTP protocol with zero port defaults to 80",
			statusMap: map[string]any{
				"listeners": []any{
					map[string]any{"name": "http"},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 0},
			},
			wantPorts: map[string]int64{"http": 80},
		},
		{
			name: "HTTPS protocol with zero port defaults to 443",
			statusMap: map[string]any{
				"listeners": []any{
					map[string]any{"name": "https"},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "https", Protocol: gwv1.HTTPSProtocolType, Port: 0},
			},
			wantPorts: map[string]int64{"https": 443},
		},
		{
			name: "unknown protocol with zero port falls back to legacyPortFallback",
			statusMap: map[string]any{
				"listeners": []any{
					map[string]any{"name": "unknown"},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "unknown", Protocol: gwv1.ProtocolType("unknown"), Port: 0},
			},
			wantPorts: map[string]int64{"unknown": legacyPortFallback},
		},
		{
			name: "multiple listeners matched by name",
			statusMap: map[string]any{
				"listeners": []any{
					map[string]any{"name": "http"},
					map[string]any{"name": "https"},
					map[string]any{"name": "admin"},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 0},
				{Name: "https", Protocol: gwv1.HTTPSProtocolType, Port: 0},
				{Name: "admin", Protocol: gwv1.HTTPProtocolType, Port: 9090},
			},
			wantPorts: map[string]int64{
				"http":  80,
				"https": 443,
				"admin": 9090,
			},
		},
		{
			name: "status listener with no matching spec entry receives fallback port",
			statusMap: map[string]any{
				"listeners": []any{
					map[string]any{"name": "orphan"},
				},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "other", Protocol: gwv1.HTTPProtocolType, Port: 80},
			},
			wantPorts: map[string]int64{"orphan": legacyPortFallback},
		},
		{
			name: "missing listeners key in statusMap is a no-op",
			statusMap: map[string]any{
				"conditions": []any{},
			},
			specListeners: []gwv1.ListenerEntry{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80},
			},
			wantPorts: map[string]int64{},
		},
		{
			name:          "empty listeners slice is a no-op",
			statusMap:     map[string]any{"listeners": []any{}},
			specListeners: []gwv1.ListenerEntry{},
			wantPorts:     map[string]int64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			injectListenerPorts(tt.statusMap, tt.specListeners)

			listeners, _ := tt.statusMap["listeners"].([]any)
			for _, entry := range listeners {
				entryMap, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				name, _ := entryMap["name"].(string)
				wantPort, shouldHavePort := tt.wantPorts[name]
				if shouldHavePort {
					got, hasPort := entryMap["port"]
					assert.True(t, hasPort, "listener %q should have port injected", name)
					assert.Equal(t, wantPort, got, "listener %q port mismatch", name)
				} else {
					_, hasPort := entryMap["port"]
					assert.False(t, hasPort, "listener %q should NOT have port injected", name)
				}
			}
		})
	}
}
