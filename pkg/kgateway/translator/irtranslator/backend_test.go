package irtranslator_test

import (
	"context"
	"errors"
	"testing"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoycommondnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/common/dns/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	sockets_raw_buffer "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/raw_buffer/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

func TestBackendTranslatorTranslatesAppProtocol(t *testing.T) {
	var bt irtranslator.BackendTranslator
	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "group",
			Kind:      "kind",
			Name:      "name",
			Namespace: "namespace",
		},
		AppProtocol: ir.HTTP2AppProtocol,
	}
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}

	c, err := bt.TranslateBackend(context.Background(), kctx, ucc, backend)
	require.NoError(t, err)
	opts := c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
	assert.NotNil(t, opts)

	p, err := opts.UnmarshalNew()
	require.NoError(t, err)

	httpOpts, ok := p.(*envoy_upstreams_v3.HttpProtocolOptions)
	assert.True(t, ok)
	assert.NotNil(t, httpOpts.GetExplicitHttpConfig().GetHttp2ProtocolOptions())
}

func TestBackendTranslatorAppliesDnsLookupFamilyToDnsCluster(t *testing.T) {
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "group",
			Kind:      "kind",
			Name:      "name",
			Namespace: "namespace",
		},
	}

	var bt irtranslator.BackendTranslator
	bt.CommonCols = &collections.CommonCollections{
		Settings: apisettings.Settings{
			DnsLookupFamily: apisettings.DnsLookupFamilyV4Only,
		},
	}
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				dnsClusterCfg, err := utils.MessageToAny(&envoydnsv3.DnsCluster{})
				require.NoError(t, err)
				out.ClusterDiscoveryType = &envoyclusterv3.Cluster_ClusterType{
					ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
						Name:        "envoy.clusters.dns",
						TypedConfig: dnsClusterCfg,
					},
				}
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{}

	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext

	cluster, err := bt.TranslateBackend(context.Background(), kctx, ucc, backend)
	require.NoError(t, err)

	clusterType := cluster.GetClusterType()
	require.NotNil(t, clusterType)
	var dnsCluster envoydnsv3.DnsCluster
	err = clusterType.GetTypedConfig().UnmarshalTo(&dnsCluster)
	require.NoError(t, err)
	assert.Equal(t, envoycommondnsv3.DnsLookupFamily_V4_ONLY, dnsCluster.GetDnsLookupFamily())
}

// TestBackendTranslatorHandlesBackendIRErrors validates that when the Backend IR itself
// has pre-existing errors, the translator returns a blackhole cluster and error.
func TestBackendTranslatorHandlesBackendIRErrors(t *testing.T) {
	// Create backend IR errors to simulate validation failures during IR construction.
	// No attached policies needed for this test.
	backendError1 := errors.New("invalid backend hostname")
	backendError2 := errors.New("unsupported backend protocol")
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "core",
			Kind:      "Service",
			Name:      "invalid-svc",
			Namespace: "test-ns",
		},
		Port:   80,
		Errors: []error{backendError1, backendError2},
		AttachedPolicies: ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{},
		},
	}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "core", Kind: "Service"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{}

	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	// Validate that the backend IR errors are propagated.
	cluster, err := bt.TranslateBackend(context.Background(), kctx, ucc, backend)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid backend hostname")
	assert.Contains(t, err.Error(), "unsupported backend protocol")

	// Should return a blackhole cluster when Backend IR has errors
	assert.NotNil(t, cluster)
	assert.Equal(t, "service_test-ns_invalid-svc_80", cluster.GetName())
	assert.Equal(t, envoyclusterv3.Cluster_STATIC, cluster.GetType())
	assert.Empty(t, cluster.GetLoadAssignment().GetEndpoints())

	// Backend IR errors should remain in the backend
	assert.NotEmpty(t, backend.Errors)
	assert.Contains(t, backend.Errors, backendError1)
	assert.Contains(t, backend.Errors, backendError2)
}

// TestBackendTranslatorPropagatesPolicyErrors validates that attached policy IR errors
// are propagated and result in an error return with a blackhole cluster.
func TestBackendTranslatorPropagatesPolicyErrors(t *testing.T) {
	policyError1 := errors.New("invalid TLS certificate")
	policyError2 := errors.New("invalid health check configuration")
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "group",
			Kind:      "kind",
			Name:      "name",
			Namespace: "namespace",
		},
		AttachedPolicies: ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{
				{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}: {
					{
						GroupKind: schema.GroupKind{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"},
						Errors:    []error{policyError1},
					},
				},
				{Group: "gateway-api", Kind: "BackendTLSPolicy"}: {
					{
						GroupKind: schema.GroupKind{Group: "gateway-api", Kind: "BackendTLSPolicy"},
						Errors:    []error{policyError2},
					},
				},
			},
		},
	}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{
		{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}: {
			Name: "BackendConfigPolicy",
			ProcessBackend: func(ctx context.Context, polir ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
			},
		},
		{Group: "gateway-api", Kind: "BackendTLSPolicy"}: {
			Name: "BackendTLSPolicy",
			ProcessBackend: func(ctx context.Context, polir ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
			},
		},
	}

	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	cluster, err := bt.TranslateBackend(context.Background(), kctx, ucc, backend)
	// Validate that the policy errors are propagated.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid TLS certificate")
	assert.Contains(t, err.Error(), "invalid health check configuration")

	// Validate that a blackhole cluster is returned when policy errors occur.
	assert.NotNil(t, cluster)
	assert.Equal(t, "kind_namespace_name_0", cluster.GetName())
	assert.Equal(t, envoyclusterv3.Cluster_STATIC, cluster.GetType())
	assert.Empty(t, cluster.GetLoadAssignment().GetEndpoints())

	// Validate that policy errors are not stored in backend.errors
	assert.Empty(t, backend.Errors)
}

// TestBackendTranslatorHandlesXDSValidationErrors validates that when xDS validation fails
// in strict mode, the translator returns a blackhole cluster and error.
func TestBackendTranslatorHandlesXDSValidationErrors(t *testing.T) {
	// Create a mock validator that always returns an error
	mockValidator := &mockValidator{
		validateFunc: func(ctx context.Context, config *envoybootstrapv3.Bootstrap) error {
			return errors.New("envoy validation failed: invalid cluster configuration")
		},
	}

	// BackendIR with no errors.
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "core",
			Kind:      "Service",
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Port: 80,
		// No pre-existing errors
		Errors: nil,
		// No attached policies that would cause errors
		AttachedPolicies: ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{},
		},
	}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "core", Kind: "Service"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{}

	// Set up strict mode and inject the mock validator
	bt.Mode = apisettings.ValidationStrict
	bt.Validator = mockValidator

	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	cluster, err := bt.TranslateBackend(context.Background(), kctx, ucc, backend)

	// Should get an error because xDS validation failed
	require.Error(t, err)
	assert.Contains(t, err.Error(), "envoy validation failed")
	assert.Contains(t, err.Error(), "invalid cluster configuration")

	// Should return a blackhole cluster when xDS validation fails
	assert.NotNil(t, cluster)
	assert.Equal(t, "service_test-ns_test-svc_80", cluster.GetName())
	assert.Equal(t, envoyclusterv3.Cluster_STATIC, cluster.GetType())
	assert.Empty(t, cluster.GetLoadAssignment().GetEndpoints())

	// Backend IR should remain clean (xDS errors don't modify backend.errors)
	assert.Empty(t, backend.Errors)
}

func TestBackendTranslatorAppliesGatewayBackendClientCertificate(t *testing.T) {
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "group",
			Kind:      "kind",
			Name:      "name",
			Namespace: "namespace",
		},
		GatewayBackendClientCertificate: &ir.GatewayBackendClientCertificateIR{
			Certificate: ir.TLSCertificate{
				CertChain:  []byte("gateway-cert"),
				PrivateKey: []byte("gateway-key"),
			},
		},
		AttachedPolicies: ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{
				{Group: "gateway-api", Kind: "BackendTLSPolicy"}: {
					{
						GroupKind: schema.GroupKind{Group: "gateway-api", Kind: "BackendTLSPolicy"},
						PolicyIr:  new(testPolicyIR),
					},
				},
			},
		},
	}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{
		{Group: "gateway-api", Kind: "BackendTLSPolicy"}: {
			ProcessBackend: func(ctx context.Context, polir ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
				typedConfig, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
					Sni: "backend.example.com",
					CommonTlsContext: &envoytlsv3.CommonTlsContext{
						ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{},
					},
				})
				require.NoError(t, err)
				out.TransportSocket = &envoycorev3.TransportSocket{
					Name: envoywellknown.TransportSocketTls,
					ConfigType: &envoycorev3.TransportSocket_TypedConfig{
						TypedConfig: typedConfig,
					},
				}
			},
		},
	}

	cluster, err := bt.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ir.UniqlyConnectedClient{}, backend)
	require.NoError(t, err)
	require.NotNil(t, cluster)
	require.NotNil(t, cluster.TransportSocket)

	tlsContext := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, cluster.TransportSocket.GetTypedConfig().UnmarshalTo(tlsContext))
	require.Len(t, tlsContext.GetCommonTlsContext().GetTlsCertificates(), 1)
	assert.Equal(t, "backend.example.com", tlsContext.GetSni())
	assert.Equal(t, "gateway-cert", tlsContext.GetCommonTlsContext().GetTlsCertificates()[0].GetCertificateChain().GetInlineString())
	assert.Equal(t, "gateway-key", tlsContext.GetCommonTlsContext().GetTlsCertificates()[0].GetPrivateKey().GetInlineString())
}

func TestBackendTranslatorDoesNotEnableTLSForGatewayBackendClientCertificate(t *testing.T) {
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "group",
			Kind:      "kind",
			Name:      "name",
			Namespace: "namespace",
		},
		GatewayBackendClientCertificate: &ir.GatewayBackendClientCertificateIR{
			Certificate: ir.TLSCertificate{
				CertChain:  []byte("gateway-cert"),
				PrivateKey: []byte("gateway-key"),
			},
		},
		AttachedPolicies: ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{},
		},
	}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{}

	cluster, err := bt.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ir.UniqlyConnectedClient{}, backend)
	require.NoError(t, err)
	require.NotNil(t, cluster)
	assert.Nil(t, cluster.TransportSocket)
}

func TestBackendTranslatorAppliesGatewayBackendClientCertificateToTransportSocketMatches(t *testing.T) {
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "group",
			Kind:      "kind",
			Name:      "name",
			Namespace: "namespace",
		},
		GatewayBackendClientCertificate: &ir.GatewayBackendClientCertificateIR{
			Certificate: ir.TLSCertificate{
				CertChain:  []byte("gateway-cert"),
				PrivateKey: []byte("gateway-key"),
			},
		},
		AttachedPolicies: ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{
				{Group: "istio.io", Kind: "Settings"}: {
					{
						GroupKind: schema.GroupKind{Group: "istio.io", Kind: "Settings"},
						PolicyIr:  new(testPolicyIR),
					},
				},
			},
		},
	}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{
		{Group: "istio.io", Kind: "Settings"}: {
			ProcessBackend: func(ctx context.Context, polir ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
				tlsTypedConfig, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
					Sni: "backend.example.com",
					CommonTlsContext: &envoytlsv3.CommonTlsContext{
						ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{},
						TlsCertificateSdsSecretConfigs: []*envoytlsv3.SdsSecretConfig{
							{Name: "existing-sds-secret"},
						},
					},
				})
				require.NoError(t, err)

				rawBufferTypedConfig, err := utils.MessageToAny(&sockets_raw_buffer.RawBuffer{})
				require.NoError(t, err)

				out.TransportSocketMatches = []*envoyclusterv3.Cluster_TransportSocketMatch{
					{
						Name: "tls-mode-istio",
						TransportSocket: &envoycorev3.TransportSocket{
							Name: envoywellknown.TransportSocketTls,
							ConfigType: &envoycorev3.TransportSocket_TypedConfig{
								TypedConfig: tlsTypedConfig,
							},
						},
					},
					{
						Name: "tls-mode-disabled",
						TransportSocket: &envoycorev3.TransportSocket{
							Name: envoywellknown.TransportSocketRawBuffer,
							ConfigType: &envoycorev3.TransportSocket_TypedConfig{
								TypedConfig: rawBufferTypedConfig,
							},
						},
					},
				}
			},
		},
	}

	cluster, err := bt.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ir.UniqlyConnectedClient{}, backend)
	require.NoError(t, err)
	require.NotNil(t, cluster)
	require.Nil(t, cluster.TransportSocket)
	require.Len(t, cluster.TransportSocketMatches, 2)

	tlsContext := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, cluster.TransportSocketMatches[0].GetTransportSocket().GetTypedConfig().UnmarshalTo(tlsContext))
	require.Len(t, tlsContext.GetCommonTlsContext().GetTlsCertificates(), 1)
	assert.Equal(t, "backend.example.com", tlsContext.GetSni())
	assert.Equal(t, "gateway-cert", tlsContext.GetCommonTlsContext().GetTlsCertificates()[0].GetCertificateChain().GetInlineString())
	assert.Equal(t, "gateway-key", tlsContext.GetCommonTlsContext().GetTlsCertificates()[0].GetPrivateKey().GetInlineString())
	assert.Nil(t, tlsContext.GetCommonTlsContext().GetTlsCertificateSdsSecretConfigs())
	assert.Equal(t, envoywellknown.TransportSocketRawBuffer, cluster.TransportSocketMatches[1].GetTransportSocket().GetName())
}

// mockValidator is a test implementation of validator.Validator for testing xDS validation errors
type mockValidator struct {
	validateFunc func(ctx context.Context, config *envoybootstrapv3.Bootstrap) error
}

var _ validator.Validator = &mockValidator{}

func (m *mockValidator) Validate(ctx context.Context, config *envoybootstrapv3.Bootstrap) error {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, config)
	}
	return nil
}

type testPolicyIR struct{}

func (t *testPolicyIR) CreationTime() time.Time {
	return time.Time{}
}

func (t *testPolicyIR) Equals(other any) bool {
	_, ok := other.(*testPolicyIR)
	return ok
}
