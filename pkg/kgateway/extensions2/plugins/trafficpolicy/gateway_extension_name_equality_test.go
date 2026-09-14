package trafficpolicy

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
)

func gatewayExtensionIRForNameTest(name string) TrafficPolicyGatewayExtensionIR {
	return TrafficPolicyGatewayExtensionIR{
		Name: name,
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
	}
}

// TestGatewayExtensionIREqualsDetectsNameOnlyChange is a regression test: Equals
// used to skip Name, so two extensions with identical provider config but
// different names compared equal. The name is not cosmetic - policy IRs embed a
// *TrafficPolicyGatewayExtensionIR and delegate to this Equals, and providerName()
// turns the name into the ext_proc/ext_authz filter name and the per-route
// typed-config key, so identical-but-renamed extensions yield different Envoy
// config.
func TestGatewayExtensionIREqualsDetectsNameOnlyChange(t *testing.T) {
	a := gatewayExtensionIRForNameTest("default/my-extension")
	b := gatewayExtensionIRForNameTest("default/renamed-extension")

	if a.Equals(b) {
		t.Error("Equals returned true for extensions that differ only by name; the name reaches Envoy as the filter name")
	}
	if !a.Equals(gatewayExtensionIRForNameTest("default/my-extension")) {
		t.Error("Equals returned false for two identical extensions")
	}
}
