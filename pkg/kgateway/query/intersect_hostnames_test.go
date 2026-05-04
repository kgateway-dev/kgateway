package query_test

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
)

func TestIntersectHostnames(t *testing.T) {
	tests := []struct {
		name             string
		listenerHostname string
		routeHostname    string
		want             string
		wantOK           bool
	}{
		{
			name:             "both empty",
			listenerHostname: "",
			routeHostname:    "",
			want:             "",
			wantOK:           true,
		},
		{
			name:             "empty listener returns route",
			listenerHostname: "",
			routeHostname:    "foo.example.com",
			want:             "foo.example.com",
			wantOK:           true,
		},
		{
			name:             "empty route returns listener",
			listenerHostname: "foo.example.com",
			routeHostname:    "",
			want:             "foo.example.com",
			wantOK:           true,
		},
		{
			name:             "exact match",
			listenerHostname: "foo.example.com",
			routeHostname:    "foo.example.com",
			want:             "foo.example.com",
			wantOK:           true,
		},
		{
			name:             "exact mismatch",
			listenerHostname: "foo.example.com",
			routeHostname:    "bar.example.com",
			want:             "foo.example.com",
			wantOK:           false,
		},
		{
			name:             "listener wildcard matches route exact",
			listenerHostname: "*.example.com",
			routeHostname:    "foo.example.com",
			want:             "foo.example.com",
			wantOK:           true,
		},
		{
			name:             "listener wildcard matches deeper route exact",
			listenerHostname: "*.example.com",
			routeHostname:    "a.b.example.com",
			want:             "a.b.example.com",
			wantOK:           true,
		},
		{
			name:             "listener wildcard does not match apex route",
			listenerHostname: "*.example.com",
			routeHostname:    "example.com",
			want:             "",
			wantOK:           false,
		},
		{
			name:             "listener wildcard does not match unrelated route",
			listenerHostname: "*.example.com",
			routeHostname:    "foo.other.com",
			want:             "",
			wantOK:           false,
		},
		{
			name:             "route wildcard matches listener exact",
			listenerHostname: "foo.example.com",
			routeHostname:    "*.example.com",
			want:             "foo.example.com",
			wantOK:           true,
		},
		{
			name:             "route wildcard does not match listener apex",
			listenerHostname: "example.com",
			routeHostname:    "*.example.com",
			want:             "",
			wantOK:           false,
		},
		{
			name:             "route wildcard does not match unrelated listener",
			listenerHostname: "foo.other.com",
			routeHostname:    "*.example.com",
			want:             "",
			wantOK:           false,
		},
		{
			name:             "both wildcards equal",
			listenerHostname: "*.example.com",
			routeHostname:    "*.example.com",
			want:             "*.example.com",
			wantOK:           true,
		},
		{
			name:             "both wildcards listener more specific",
			listenerHostname: "*.foo.example.com",
			routeHostname:    "*.example.com",
			want:             "*.foo.example.com",
			wantOK:           true,
		},
		{
			name:             "both wildcards route more specific",
			listenerHostname: "*.example.com",
			routeHostname:    "*.foo.example.com",
			want:             "*.foo.example.com",
			wantOK:           true,
		},
		{
			name:             "both wildcards no overlap",
			listenerHostname: "*.example.com",
			routeHostname:    "*.other.com",
			want:             "",
			wantOK:           false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := query.IntersectHostnames(tc.listenerHostname, tc.routeHostname)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("IntersectHostnames(%q, %q) = (%q, %v), want (%q, %v)",
					tc.listenerHostname, tc.routeHostname, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
