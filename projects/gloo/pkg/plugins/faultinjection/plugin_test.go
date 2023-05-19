package faultinjection

import (
	"reflect"
	"strings"
	"testing"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/internal/common"
)

func TestToEnvoyPercentage(t *testing.T) {
	assertEqualPercent(.0001, 1, t) // from the docs
	assertEqualPercent(1, 10000, t)
	assertEqualPercent(50.0005, 500005, t)
	assertEqualPercent(100, 1000000, t)
	// assertEqualPercent(50.000005, 50000005, t) cannot test for this level of precision
}

func assertEqualPercent(actual float32, expectedNumerator uint32, t *testing.T) {
	expectedPercentage := envoytype.FractionalPercent{
		Numerator:   expectedNumerator,
		Denominator: envoytype.FractionalPercent_MILLION,
	}

	actualPercentage := common.ToEnvoyPercentage(actual)
	if !reflect.DeepEqual(expectedPercentage, *actualPercentage) {
		t.Errorf("Expected %v but got %v.", expectedPercentage, actualPercentage)
	}
}

// TestProcessRoute is a minimal approach to checking that we
// catch the appropriate invalid configuration.
func TestProcessRoute(t *testing.T) {
	// Setup an instance of the plugin
	p := NewPlugin()
	p.Init(plugins.InitParams{})

	out := &envoy_config_route_v3.Route{
		Action: &envoy_config_route_v3.Route_Route{
			Route: &envoy_config_route_v3.RouteAction{
				PrefixRewrite: "/",
			},
		},
	}
	routeAct := &v1.Route_RouteAction{
		RouteAction: &v1.RouteAction{
			Destination: &v1.RouteAction_Single{
				Single: &v1.Destination{},
			},
		},
	}

	tests := []struct {
		description string
		fault       faultinjection.RouteFaults
		errContains string
	}{
		{"no faults", faultinjection.RouteFaults{}, ""},
		{"delay too short", faultinjection.RouteFaults{Delay: &faultinjection.RouteDelay{FixedDelay: &duration.Duration{Seconds: 0}}}, " must be greater than 0"},
		{"delay non-zero but short", faultinjection.RouteFaults{Delay: &faultinjection.RouteDelay{FixedDelay: &duration.Duration{Nanos: 1}}}, "invalid delay duration 'nanos:1'"},

		{"empty abort", faultinjection.RouteFaults{Abort: &faultinjection.RouteAbort{}}, "status code '0', must be in range of [200,600)"},
	}
	for _, tc := range tests {
		// not safe to run parallel but no big deal still make local copy
		tc := tc
		t.Run(tc.description, func(t *testing.T) {

			err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
				Options: &v1.RouteOptions{
					Faults:        &tc.fault,
					PrefixRewrite: &wrappers.StringValue{Value: ""},
				},
				Action: routeAct,
			}, out)
			if tc.errContains == "" {
				if err != nil {
					t.Fatalf(" Test:%v, Expected no error but got %v.", tc.description, err)
				}
				return

			}
			if err == nil {
				t.Fatalf("Test:%v, Expected error but got none", tc.description)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("Test:%v, Expected error to contain %v but got %v.", tc.description, tc.errContains, err)
			}
		})
	}

}
