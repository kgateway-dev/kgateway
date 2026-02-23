package trafficpolicy

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestIsGRPCRoute(t *testing.T) {
	tests := []struct {
		name      string
		routeName string
		want      bool
	}{
		{
			name:      "GRPCRoute with vhost prefix",
			routeName: "listener~80~example_com-route-0-grpcroute-example-grpc-route-default-0-0-matcher-0",
			want:      true,
		},
		{
			name:      "HTTPRoute with vhost prefix",
			routeName: "listener~80~example_com-route-0-httproute-example-route-default-0-0-matcher-0",
			want:      false,
		},
		{
			name:      "empty route name",
			routeName: "",
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := &envoyroutev3.Route{Name: tt.routeName}
			assert.Equal(t, tt.want, isGRPCRoute(route))
		})
	}
}

func TestApplyTimeoutDefaults(t *testing.T) {
	thirtySeconds := durationpb.New(30_000_000_000) // 30s
	sixtySeconds := durationpb.New(60_000_000_000)  // 60s
	tenSeconds := durationpb.New(10_000_000_000)    // 10s

	tests := []struct {
		name     string
		routes   []*envoyroutev3.Route
		timeouts *timeoutsIR
		// expected timeout/idleTimeout per route (nil means not set)
		wantTimeout     []*durationpb.Duration
		wantIdleTimeout []*durationpb.Duration
	}{
		{
			name: "nil timeouts is a no-op",
			routes: []*envoyroutev3.Route{
				{
					Name:   "listener~80~example_com-route-0-httproute-example-default-0-0-matcher-0",
					Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
				},
			},
			timeouts:        nil,
			wantTimeout:     []*durationpb.Duration{nil},
			wantIdleTimeout: []*durationpb.Duration{nil},
		},
		{
			name: "applies defaults to HTTPRoute without existing timeouts",
			routes: []*envoyroutev3.Route{
				{
					Name:   "listener~80~example_com-route-0-httproute-example-default-0-0-matcher-0",
					Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
				},
			},
			timeouts: &timeoutsIR{
				routeTimeout:           thirtySeconds,
				routeStreamIdleTimeout: sixtySeconds,
			},
			wantTimeout:     []*durationpb.Duration{thirtySeconds},
			wantIdleTimeout: []*durationpb.Duration{sixtySeconds},
		},
		{
			name: "does not override existing timeouts on HTTPRoute",
			routes: []*envoyroutev3.Route{
				{
					Name: "listener~80~example_com-route-0-httproute-example-default-0-0-matcher-0",
					Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
						Timeout:     tenSeconds,
						IdleTimeout: tenSeconds,
					}},
				},
			},
			timeouts: &timeoutsIR{
				routeTimeout:           thirtySeconds,
				routeStreamIdleTimeout: sixtySeconds,
			},
			wantTimeout:     []*durationpb.Duration{tenSeconds},
			wantIdleTimeout: []*durationpb.Duration{tenSeconds},
		},
		{
			name: "skips GRPCRoute — does not apply gateway-level defaults",
			routes: []*envoyroutev3.Route{
				{
					Name:   "listener~80~example_com-route-0-grpcroute-example-grpc-route-default-0-0-matcher-0",
					Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
				},
			},
			timeouts: &timeoutsIR{
				routeTimeout:           thirtySeconds,
				routeStreamIdleTimeout: sixtySeconds,
			},
			wantTimeout:     []*durationpb.Duration{nil},
			wantIdleTimeout: []*durationpb.Duration{nil},
		},
		{
			name: "mixed routes — applies to HTTPRoute but skips GRPCRoute",
			routes: []*envoyroutev3.Route{
				{
					Name:   "listener~80~example_com-route-0-grpcroute-example-grpc-route-default-0-0-matcher-0",
					Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
				},
				{
					Name:   "listener~80~example_com-route-1-httproute-example-route-default-0-0-matcher-0",
					Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{}},
				},
			},
			timeouts: &timeoutsIR{
				routeTimeout:           thirtySeconds,
				routeStreamIdleTimeout: sixtySeconds,
			},
			wantTimeout:     []*durationpb.Duration{nil, thirtySeconds},
			wantIdleTimeout: []*durationpb.Duration{nil, sixtySeconds},
		},
		{
			name: "skips routes without RouteAction",
			routes: []*envoyroutev3.Route{
				{
					Name:   "listener~80~example_com-route-0-httproute-example-default-0-0-matcher-0",
					Action: &envoyroutev3.Route_Redirect{},
				},
			},
			timeouts: &timeoutsIR{
				routeTimeout:           thirtySeconds,
				routeStreamIdleTimeout: sixtySeconds,
			},
			wantTimeout:     []*durationpb.Duration{nil},
			wantIdleTimeout: []*durationpb.Duration{nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			applyTimeoutDefaults(tt.routes, tt.timeouts)

			for i, route := range tt.routes {
				action := route.GetRoute()
				if tt.wantTimeout[i] == nil {
					a.Nil(action.GetTimeout(), "route %d timeout should be nil", i)
				} else {
					a.Equal(tt.wantTimeout[i].AsDuration(), action.GetTimeout().AsDuration(), "route %d timeout mismatch", i)
				}
				if tt.wantIdleTimeout[i] == nil {
					a.Nil(action.GetIdleTimeout(), "route %d idleTimeout should be nil", i)
				} else {
					a.Equal(tt.wantIdleTimeout[i].AsDuration(), action.GetIdleTimeout().AsDuration(), "route %d idleTimeout mismatch", i)
				}
			}
		})
	}
}
