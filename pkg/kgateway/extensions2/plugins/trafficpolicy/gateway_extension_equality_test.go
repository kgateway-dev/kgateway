package trafficpolicy

import (
	"errors"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoymatchingv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/matching/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	ratev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

func baseHarnessGatewayExtensionIR() TrafficPolicyGatewayExtensionIR {
	return TrafficPolicyGatewayExtensionIR{
		Name: "default/my-extension",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{
			Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
				GrpcService: &envoycorev3.GrpcService{
					TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{ClusterName: "ext-authz-cluster"},
					},
				},
			},
		},
		PrecedenceWeight: 5,
		FilterStage: &kgateway.FilterStageSpec{
			Stage:     kgateway.FilterStageAuthZ,
			Predicate: kgateway.FilterStagePredicateAfter,
			Weight:    1,
		},
	}
}

// TestHarnessTrafficPolicyGatewayExtensionIREquals pins that every field of the
// extension IR participates in equality. Name is not cosmetic: Equals used to skip
// it, yet policy IRs embed a *TrafficPolicyGatewayExtensionIR and delegate to this
// Equals, and providerName() turns the name into the ext_proc/ext_authz filter name
// and the per-route typed-config key.
func TestHarnessTrafficPolicyGatewayExtensionIREquals(t *testing.T) {
	cases := []equalstest.Case[TrafficPolicyGatewayExtensionIR]{
		{
			Field:  "Name",
			Mutate: func(e *TrafficPolicyGatewayExtensionIR) { e.Name = "default/other-extension" },
		},
		{
			Field: "ExtAuth",
			Mutate: func(e *TrafficPolicyGatewayExtensionIR) {
				e.ExtAuth = &envoy_ext_authz_v3.ExtAuthz{
					Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
						GrpcService: &envoycorev3.GrpcService{
							TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
								EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{ClusterName: "other-cluster"},
							},
						},
					},
				}
			},
		},
		{
			Field: "ExtProc",
			Mutate: func(e *TrafficPolicyGatewayExtensionIR) {
				e.ExtProc = buildCompositeExtProcFilter(
					kgateway.ExtProcProvider{FailOpen: true},
					&envoycorev3.GrpcService{
						TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{ClusterName: "ext-proc-cluster"},
						},
					},
				)
			},
		},
		{
			Field: "RateLimit",
			Mutate: func(e *TrafficPolicyGatewayExtensionIR) {
				e.RateLimit = &ratev3.RateLimit{Domain: "my-domain"}
			},
		},
		{
			Field: "Jwt",
			Mutate: func(e *TrafficPolicyGatewayExtensionIR) {
				e.Jwt = &envoymatchingv3.ExtensionWithMatcher{}
			},
		},
		{
			Field: "OAuth2",
			Mutate: func(e *TrafficPolicyGatewayExtensionIR) {
				e.OAuth2 = &oauthPerProviderConfig{}
			},
		},
		{
			Field:  "PrecedenceWeight",
			Mutate: func(e *TrafficPolicyGatewayExtensionIR) { e.PrecedenceWeight = 10 },
		},
		{
			Field: "FilterStage",
			Mutate: func(e *TrafficPolicyGatewayExtensionIR) {
				e.FilterStage = &kgateway.FilterStageSpec{
					Stage:     kgateway.FilterStageAuthZ,
					Predicate: kgateway.FilterStagePredicateBefore,
					Weight:    1,
				}
			},
		},
		{
			Field:  "Err",
			Mutate: func(e *TrafficPolicyGatewayExtensionIR) { e.Err = errors.New("resolution failed") },
		},
	}

	equalstest.Run(
		t,
		baseHarnessGatewayExtensionIR,
		func(a, b TrafficPolicyGatewayExtensionIR) bool { return a.Equals(b) },
		cases,
		nil,
	)
}

// TestGatewayExtensionIREqualsNilFilterStage covers what the harness cannot: an
// unset FilterStage must equal another unset one and differ from a set one.
func TestGatewayExtensionIREqualsNilFilterStage(t *testing.T) {
	set := baseHarnessGatewayExtensionIR()
	unset := baseHarnessGatewayExtensionIR()
	unset.FilterStage = nil

	if set.Equals(unset) || unset.Equals(set) {
		t.Error("Equals returned true comparing a set FilterStage against nil")
	}
	alsoUnset := baseHarnessGatewayExtensionIR()
	alsoUnset.FilterStage = nil
	if !unset.Equals(alsoUnset) {
		t.Error("Equals returned false for two extensions that both have a nil FilterStage")
	}
}
