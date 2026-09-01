package statusscale_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestIsGatewayAPIStatusWrite(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "route status", method: http.MethodPut, path: "/apis/gateway.networking.k8s.io/v1/namespaces/gwtest/httproutes/r/status", want: true},
		{name: "gateway status", method: http.MethodPatch, path: "/apis/gateway.networking.k8s.io/v1/namespaces/gwtest/gateways/g/status", want: true},
		{name: "route spec", method: http.MethodPut, path: "/apis/gateway.networking.k8s.io/v1/namespaces/gwtest/httproutes/r"},
		{name: "read status", method: http.MethodGet, path: "/apis/gateway.networking.k8s.io/v1/namespaces/gwtest/httproutes/r/status"},
		{name: "core status", method: http.MethodPut, path: "/api/v1/namespaces/gwtest/pods/p/status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{Method: tt.method, URL: &url.URL{Path: tt.path}}
			if got := isGatewayAPIStatusWrite(req); got != tt.want {
				t.Fatalf("isGatewayAPIStatusWrite() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadMetricsSince(t *testing.T) {
	base := time.Unix(100, 0)
	p := &statusWriteProbe{events: []statusWriteEvent{
		{start: base, finish: base.Add(time.Second), statusCode: http.StatusOK},
		{start: base.Add(time.Second), finish: base.Add(2 * time.Second), statusCode: http.StatusConflict},
		{start: base.Add(2 * time.Second), finish: base.Add(4 * time.Second), statusCode: http.StatusNoContent},
	}}
	staleness := make([]time.Duration, 100)
	for i := range staleness {
		staleness[i] = time.Duration(i+1) * time.Second
	}

	got := p.loadMetricsSince(0, 10*time.Second, staleness)
	if got.WriteAttempts != 3 || got.SuccessfulWrites != 2 || got.Conflicts != 1 {
		t.Fatalf("unexpected write counts: %+v", got)
	}
	if got.WriteActiveSeconds != 4 || got.WriteQPS != 0.5 {
		t.Fatalf("unexpected write rate: %+v", got)
	}
	if got.P95StalenessSeconds != 95 || got.MaxStalenessSeconds != 100 {
		t.Fatalf("unexpected staleness: %+v", got)
	}
}
