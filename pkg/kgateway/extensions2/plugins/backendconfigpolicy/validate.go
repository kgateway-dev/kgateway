package backendconfigpolicy

import (
	"context"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
	"github.com/kgateway-dev/kgateway/v2/pkg/xds/bootstrap"
)

const strictValidationPlaceholderCACert = `-----BEGIN CERTIFICATE-----
MIIC1jCCAb4CCQCJczLyBBZ1GTANBgkqhkiG9w0BAQsFADAtMRUwEwYDVQQKDAxl
eGFtcGxlIEluYy4xFDASBgNVBAMMC2V4YW1wbGUuY29tMB4XDTI1MDMwNzE0Mjkx
NloXDTI2MDMwNzE0MjkxNlowLTEVMBMGA1UECgwMZXhhbXBsZSBJbmMuMRQwEgYD
VQQDDAtleGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AN0U6TVYECkwqnxh1Kt3dS+LialrXBOXKagj9tE582T6dwmqThD75VZPrNKkRoYO
aUzCctfDkUBXRemOTMut7ES5xoAtSAhr2GAnqgM3+yBCLOxooSjEFdlpFT7dhi1w
jOPa5iMh6ve/pHuRHvEuaF/J6P8tr83wGutx/xFZVuGA9V1AmBmYhePM+JhdcwaB
1+IbJp30gGyPfY4vdRQ9VQWbThE8psEzah+3SgTKJSIT7NAdwiIu3O3rXORbaYYU
oycgXUHdOKRbJnbvy3pTnFZJ50sg1HIA4yBdX7c0diy8Zz3Suoondg3DforWr0pB
Hs6tySAQoz2RiAqDqcE2rbMCAwEAATANBgkqhkiG9w0BAQsFAAOCAQEAWPkz3dJW
b+LFtnv7MlOVM79Y4PqeiHnazP1G9FwnWBHARkjISsax3b0zX8/RHnU83c3tLP5D
VwenYb9B9mzXbLiWI8aaX0UXP//D593ti15y0Od7yC2hQszlqIbxYnkFVwXoT9fQ
bdQ9OtpCt8EZnKEyCxck+hlKEyYTcH2PqZ7Ndp0M8I2znz3Kut/uYHLUddfoPF/m
O0V6fbyB/Mx/G1uLiv/BVpx3AdP+3ygJyKtelXkD+IdlY3y110fzmVr6NgxAbz/h
n9KpuK4SEloIycZUaKVXAaX7T42SFYw7msmB+Uu7z5oLOijsjX6TjeofdFBZ/Byl
SxODgqhtaPnOxQ==
-----END CERTIFICATE-----`

// validateXDS performs xDS validation checks on the BCP IR definition. This acts as a
// safety net to catch bugs in the IR construction logic and prevents invalid configuration
// from being applied when STRICT mode is enabled.
func validateXDS(
	ctx context.Context,
	policyIR *BackendConfigPolicyIR,
	v validator.Validator,
	mode apisettings.ValidationMode,
) error {
	if mode != apisettings.ValidationStrict || v == nil {
		return nil
	}

	testCluster := &envoyclusterv3.Cluster{
		Name:                 "test-cluster-for-validation",
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC},
		ConnectTimeout:       durationpb.New(5 * time.Second),
	}
	if requiresDnsClusterValidation(policyIR) {
		dnsClusterConfig, err := utils.MessageToAny(&envoydnsv3.DnsCluster{})
		if err != nil {
			return err
		}
		testCluster.ClusterDiscoveryType = &envoyclusterv3.Cluster_ClusterType{
			ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
				Name:        dnsClusterExtensionName,
				TypedConfig: dnsClusterConfig,
			},
		}
	}
	dummyBackend := ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "core",
			Kind:      "Service",
			Name:      "test-backend",
			Namespace: "test",
		},
		Port: 80,
	}
	processBackend(ctx, policyIR, dummyBackend, testCluster)

	builder := bootstrap.New()
	builder.AddCluster(testCluster)
	if requiresSystemCASecretValidation(policyIR) {
		builder.AddSecret(systemCAValidationSecret())
	}
	bootstrap, err := builder.Build()
	if err != nil {
		return err
	}

	return v.Validate(ctx, bootstrap)
}

func requiresDnsClusterValidation(policyIR *BackendConfigPolicyIR) bool {
	return (policyIR.loadBalancerConfig != nil && policyIR.loadBalancerConfig.useHostnameForHashing) ||
		policyIR.dnsRefreshRate != nil ||
		policyIR.dnsJitter != nil ||
		policyIR.respectDnsTtl != nil
}

func requiresSystemCASecretValidation(policyIR *BackendConfigPolicyIR) bool {
	if policyIR == nil || policyIR.tlsConfig == nil || policyIR.tlsConfig.CommonTlsContext == nil {
		return false
	}

	switch validation := policyIR.tlsConfig.CommonTlsContext.GetValidationContextType().(type) {
	case *envoytlsv3.CommonTlsContext_CombinedValidationContext:
		return validation.CombinedValidationContext.GetValidationContextSdsSecretConfig().GetName() == eiutils.SystemCaSecretName
	case *envoytlsv3.CommonTlsContext_ValidationContextSdsSecretConfig:
		return validation.ValidationContextSdsSecretConfig.GetName() == eiutils.SystemCaSecretName
	default:
		return false
	}
}

func systemCAValidationSecret() *envoytlsv3.Secret {
	return &envoytlsv3.Secret{
		Name: eiutils.SystemCaSecretName,
		Type: &envoytlsv3.Secret_ValidationContext{
			ValidationContext: &envoytlsv3.CertificateValidationContext{
				TrustedCa: &envoycorev3.DataSource{
					Specifier: &envoycorev3.DataSource_InlineString{
						// The STRICT validator only needs a valid placeholder secret so
						// it can validate references that envoyinit resolves at runtime.
						InlineString: strictValidationPlaceholderCACert,
					},
				},
			},
		},
	}
}
