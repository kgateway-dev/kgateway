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

func newTestBackend(objSrc ir.ObjectSource, port int32) *ir.BackendObjectIR {
	backend := ir.NewBackendObjectIR(objSrc, port, "", "")
	return &backend
}

func translateBackendBase(t *testing.T, bt *irtranslator.BackendTranslator, backend *ir.BackendObjectIR) (*envoyclusterv3.Cluster, error) {
	t.Helper()
	base := bt.TranslateBackendBase(t.Context(), backend)
	require.NotNil(t, base)
	return base.Cluster, base.Error
}

func TestBackendTranslatorTranslatesAppProtocol(t *testing.T) {
	var bt irtranslator.BackendTranslator
	backend := newTestBackend(ir.ObjectSource{
		Group:     "group",
		Kind:      "kind",
		Name:      "name",
		Namespace: "namespace",
	}, 0)
	backend.AppProtocol = ir.HTTP2AppProtocol
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}

	c, err := translateBackendBase(t, &bt, backend)
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
	backend := newTestBackend(ir.ObjectSource{
		Group:     "group",
		Kind:      "kind",
		Name:      "name",
		Namespace: "namespace",
	}, 0)

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

	cluster, err := translateBackendBase(t, &bt, backend)
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
	backend := newTestBackend(ir.ObjectSource{
		Group:     "core",
		Kind:      "Service",
		Name:      "invalid-svc",
		Namespace: "test-ns",
	}, 80)
	backend.Errors = []error{backendError1, backendError2}
	backend.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
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

	// Validate that the backend IR errors are propagated.
	cluster, err := translateBackendBase(t, &bt, backend)
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
	backend := newTestBackend(ir.ObjectSource{
		Group:     "group",
		Kind:      "kind",
		Name:      "name",
		Namespace: "namespace",
	}, 0)
	backend.AttachedPolicies = ir.AttachedPolicies{
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

	cluster, err := translateBackendBase(t, &bt, backend)
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
	backend := newTestBackend(ir.ObjectSource{
		Group:     "core",
		Kind:      "Service",
		Name:      "test-svc",
		Namespace: "test-ns",
	}, 80)
	backend.Errors = nil
	backend.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
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

	cluster, err := translateBackendBase(t, &bt, backend)

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
	backend := newTestBackend(ir.ObjectSource{
		Group:     "group",
		Kind:      "kind",
		Name:      "name",
		Namespace: "namespace",
	}, 0)
	backend.GatewayBackendClientCertificate = &ir.GatewayBackendClientCertificateIR{
		Certificate: ir.TLSCertificate{
			CertChain:  []byte("gateway-cert"),
			PrivateKey: []byte("gateway-key"),
		},
	}
	backend.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{
			{Group: "gateway-api", Kind: "BackendTLSPolicy"}: {
				{
					GroupKind: schema.GroupKind{Group: "gateway-api", Kind: "BackendTLSPolicy"},
					PolicyIr:  new(testPolicyIR),
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

	cluster, err := translateBackendBase(t, &bt, backend)
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
	backend := newTestBackend(ir.ObjectSource{
		Group:     "group",
		Kind:      "kind",
		Name:      "name",
		Namespace: "namespace",
	}, 0)
	backend.GatewayBackendClientCertificate = &ir.GatewayBackendClientCertificateIR{
		Certificate: ir.TLSCertificate{
			CertChain:  []byte("gateway-cert"),
			PrivateKey: []byte("gateway-key"),
		},
	}
	backend.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
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

	cluster, err := translateBackendBase(t, &bt, backend)
	require.NoError(t, err)
	require.NotNil(t, cluster)
	assert.Nil(t, cluster.TransportSocket)
}

func TestBackendTranslatorAppliesGatewayBackendClientCertificateToTransportSocketMatches(t *testing.T) {
	backend := newTestBackend(ir.ObjectSource{
		Group:     "group",
		Kind:      "kind",
		Name:      "name",
		Namespace: "namespace",
	}, 0)
	backend.GatewayBackendClientCertificate = &ir.GatewayBackendClientCertificateIR{
		Certificate: ir.TLSCertificate{
			CertChain:  []byte("gateway-cert"),
			PrivateKey: []byte("gateway-key"),
		},
	}
	backend.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{
			{Group: "istio.io", Kind: "Settings"}: {
				{
					GroupKind: schema.GroupKind{Group: "istio.io", Kind: "Settings"},
					PolicyIr:  new(testPolicyIR),
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

	cluster, err := translateBackendBase(t, &bt, backend)
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

// TestApplyPerClient_StrictModeRejectsInvalidOverlay is a regression test for
// the strict-mode bypass: PerClientClusterOverlay hooks (destrule, waypoint,
// …) mutate the cluster AFTER TranslateBackendBase has validated it. Without
// per-client validation, invalid overlay output would only surface as Envoy
// NACKs at the data plane. The fix runs strict-mode validation on the
// post-overlay cluster too. This test:
//
//  1. Sets up a translator where the validator rejects only clusters that have
//     an OutlierDetection set (the marker of our test overlay).
//  2. Translates the base — passes validation (no outlier).
//  3. Calls ApplyPerClient with an overlay that sets OutlierDetection.
//  4. Asserts the result is an error + the blackhole cluster.
func TestApplyPerClient_StrictModeRejectsInvalidOverlay(t *testing.T) {
	backendIR := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "core",
		Kind:      "Service",
		Name:      "svc",
		Namespace: "ns",
	}, 80, "", "")
	backendIR.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	backend := &backendIR

	overlayGK := schema.GroupKind{Group: "test", Kind: "DestructiveOverlay"}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "core", Kind: "Service"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{
		overlayGK: {
			PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
				return &sdk.ClusterOverlay{
					Mutate: func(out *envoyclusterv3.Cluster) {
						out.OutlierDetection = &envoyclusterv3.OutlierDetection{}
					},
				}
			},
		},
	}
	bt.Mode = apisettings.ValidationStrict
	bt.Validator = &mockValidator{
		validateFunc: func(ctx context.Context, config *envoybootstrapv3.Bootstrap) error {
			for _, c := range config.GetStaticResources().GetClusters() {
				if c.GetOutlierDetection() != nil {
					return errors.New("overlay produced invalid cluster")
				}
			}
			return nil
		},
	}

	ctx := context.Background()
	base := bt.TranslateBackendBase(ctx, backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error, "base must pass strict validation (no overlay applied yet)")

	cluster, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, ir.UniquelyConnectedClient{}, backend, base)
	require.Error(t, err, "strict-mode validation must reject invalid overlay output")
	assert.Contains(t, err.Error(), "overlay produced invalid cluster")
	require.NotNil(t, cluster, "must return a blackhole cluster so the snapshot can mark it errored")
	assert.Equal(t, envoyclusterv3.Cluster_STATIC, cluster.GetType(),
		"errored per-client cluster should be the blackhole STATIC cluster")
}

// TestApplyPerClient_StrictModePassesValidOverlay confirms the validator is
// invoked on overlay output but does not reject when the result is valid.
func TestApplyPerClient_StrictModePassesValidOverlay(t *testing.T) {
	backendIR := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "core",
		Kind:      "Service",
		Name:      "svc",
		Namespace: "ns",
	}, 80, "", "")
	backendIR.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	backend := &backendIR

	overlayGK := schema.GroupKind{Group: "test", Kind: "MarkerOverlay"}

	var validatorCalls int
	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "core", Kind: "Service"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{
		overlayGK: {
			PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
				return &sdk.ClusterOverlay{
					Mutate: func(out *envoyclusterv3.Cluster) {
						out.OutlierDetection = &envoyclusterv3.OutlierDetection{}
					},
				}
			},
		},
	}
	bt.Mode = apisettings.ValidationStrict
	bt.Validator = &mockValidator{
		validateFunc: func(ctx context.Context, config *envoybootstrapv3.Bootstrap) error {
			validatorCalls++
			return nil
		},
	}

	ctx := context.Background()
	base := bt.TranslateBackendBase(ctx, backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error)
	require.Equal(t, 1, validatorCalls, "base translation must invoke the validator once")

	cluster, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, ir.UniquelyConnectedClient{}, backend, base)
	require.NoError(t, err)
	require.NotNil(t, cluster)
	require.NotNil(t, cluster.OutlierDetection, "overlay mutation must be retained on the returned cluster")
	assert.Equal(t, 2, validatorCalls,
		"strict mode must invoke the validator a second time on the post-overlay cluster")
}

// TestTranslateBackendBase_StrictModeDefersValidationForInlineCLA is a
// regression test for strict-mode validation of CLA-built-per-client backends
// (e.g. ServiceEntry DNS/STATIC resolution). The base cluster carries no
// LoadAssignment — the CLA is attached per client in ApplyPerClient — and Envoy
// rejects some CLA-less clusters outright (logical-DNS semantics require exactly
// one endpoint). Validating the incomplete base would blackhole a perfectly
// valid backend for every client, so TranslateBackendBase must defer validation
// to ApplyPerClient, which validates the complete per-client cluster.
func TestTranslateBackendBase_StrictModeDefersValidationForInlineCLA(t *testing.T) {
	backendIR := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "core",
		Kind:      "Service",
		Name:      "svc",
		Namespace: "ns",
	}, 80, "", "")
	backendIR.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{},
	}
	backend := &backendIR

	var validatedClusters []*envoyclusterv3.Cluster
	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "core", Kind: "Service"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS}
				eps := ir.NewEndpointsForBackend(in)
				eps.Add(ir.PodLocality{}, ir.EndpointWithMd{LbEndpoint: pipeEndpoint("a")})
				return eps
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{}
	bt.Mode = apisettings.ValidationStrict
	// Mimics Envoy's logical-DNS check: any cluster without a load_assignment
	// is rejected. A valid ServiceEntry-style backend passed this on main only
	// because validation ran after the CLA was attached.
	bt.Validator = &mockValidator{
		validateFunc: func(ctx context.Context, config *envoybootstrapv3.Bootstrap) error {
			for _, c := range config.GetStaticResources().GetClusters() {
				validatedClusters = append(validatedClusters, c)
				if c.GetLoadAssignment() == nil {
					return errors.New("clusters must have a load_assignment")
				}
			}
			return nil
		},
	}

	ctx := context.Background()
	base := bt.TranslateBackendBase(ctx, backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error,
		"the CLA-less base must not be validated (and so must not error) — its CLA is built per client")
	require.Empty(t, validatedClusters, "validation must be deferred until the per-client CLA exists")

	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, ucc, backend, base)
	require.NoError(t, err, "the complete per-client cluster must pass validation")
	require.NotNil(t, perClient)
	require.NotNil(t, perClient.GetLoadAssignment(), "per-client cluster must carry the built CLA")
	require.Len(t, validatedClusters, 1, "strict mode must validate the complete per-client cluster exactly once")
}
