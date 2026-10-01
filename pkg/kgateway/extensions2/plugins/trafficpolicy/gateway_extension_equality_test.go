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
// extension IR participates in equality. Name matters because policy IRs embed a
// *TrafficPolicyGatewayExtensionIR and delegate to this Equals, and the name
// becomes the ext_proc/ext_authz filter name in the generated Envoy config.
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

// TestGatewayExtensionIREqualsDetectsNameAndFilterStageChanges spells out the case the old
// Equals missed: two extensions with identical config but different names produce
// different filter names in the dataplane, so they must not compare equal.
func TestGatewayExtensionIREqualsDetectsNameAndFilterStageChanges(t *testing.T) {
	a := baseHarnessGatewayExtensionIR()
	b := baseHarnessGatewayExtensionIR()
	b.Name = "default/renamed-extension"

	if a.Equals(b) {
		t.Error("Equals returned true for extensions that differ only by name; the name reaches Envoy as the filter name")
	}

	// FilterStage is compared without reflect.DeepEqual; nil handling must hold.
	nilStage := baseHarnessGatewayExtensionIR()
	nilStage.FilterStage = nil
	if a.Equals(nilStage) {
		t.Error("Equals returned true comparing a set FilterStage against nil")
	}
	if !nilStage.Equals(nilStage) {
		t.Error("Equals returned false for two extensions that both have a nil FilterStage")
	}
}
