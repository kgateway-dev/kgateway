package irtranslator_test

import (
	"context"
	"errors"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// edsBackendTranslator returns a translator whose single backend produces a
// plain EDS cluster (no inline endpoints) plus the supplied per-client policy
// plugins keyed by their GroupKind.
func edsBackendTranslator(policies map[schema.GroupKind]sdk.PolicyPlugin) *irtranslator.BackendTranslator {
	bt := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "group", Kind: "kind"}: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		ContributedPolicies: policies,
	}
	return bt
}

func overlayBackend() *ir.BackendObjectIR {
	b := newTestBackend(ir.ObjectSource{Group: "group", Kind: "kind", Name: "name", Namespace: "ns"}, 80)
	b.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{}}
	return b
}

// TestApplyPerClient_FastPathSharesBase: when no plugin contributes an overlay
// and the cluster does not need an inline CLA, ApplyPerClient returns nil so the
// caller shares the (read-only) base proto. This is the dominant path that keeps
// the per-client cluster collection sparse.
func TestApplyPerClient_FastPathSharesBase(t *testing.T) {
	bt := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{})
	backend := overlayBackend()
	ctx := context.Background()

	base := bt.TranslateBackendBase(ctx, backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error)

	perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, ir.UniquelyConnectedClient{}, backend, base)
	require.NoError(t, err)
	assert.Nil(t, perClient, "no overlay and no inline CLA must take the fast path (nil => share base)")
}

// TestApplyPerClient_DoesNotMutateBase is the central copy-on-write guard. An
// overlay that mutates the cluster for a matching UCC must not touch the shared
// base proto, and must return a distinct proto carrying the mutation. A second
// UCC the overlay declines (returns nil) takes the fast path and shares the base.
func TestApplyPerClient_DoesNotMutateBase(t *testing.T) {
	overlayGK := schema.GroupKind{Group: "test", Kind: "Overlay"}
	bt := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		overlayGK: {
			PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
				if ucc.Labels["match"] != "yes" {
					return nil
				}
				return &sdk.ClusterOverlay{
					Mutate: func(out *envoyclusterv3.Cluster) {
						out.OutlierDetection = &envoyclusterv3.OutlierDetection{}
					},
				}
			},
		},
	})
	backend := overlayBackend()
	ctx := context.Background()

	base := bt.TranslateBackendBase(ctx, backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error)
	require.Nil(t, base.Cluster.GetOutlierDetection(), "base must start without the overlay mutation")

	matching := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{"match": "yes"}, ir.PodLocality{})
	other := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{"match": "no"}, ir.PodLocality{})

	matched, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, matching, backend, base)
	require.NoError(t, err)
	require.NotNil(t, matched)
	assert.NotSame(t, base.Cluster, matched, "matching client must get its own proto, not the shared base")
	assert.NotNil(t, matched.GetOutlierDetection(), "overlay mutation must land on the returned proto")

	// The base proto must remain pristine after the overlay ran.
	assert.Nil(t, base.Cluster.GetOutlierDetection(), "overlay must not mutate the shared base proto")

	// A client the overlay declines shares the base (fast path).
	declined, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, other, backend, base)
	require.NoError(t, err)
	assert.Nil(t, declined, "non-matching client must take the fast path and share the base")
}

// TestApplyPerClient_BaseErrorIsNoOp: when the base is errored there is no
// per-client variation to compute — ApplyPerClient is a no-op so every client
// shares the single blackhole/error recorded on the base.
func TestApplyPerClient_BaseErrorIsNoOp(t *testing.T) {
	overlayGK := schema.GroupKind{Group: "test", Kind: "Overlay"}
	overlayCalls := 0
	bt := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		overlayGK: {
			PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
				overlayCalls++
				return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {}}
			},
		},
	})
	backend := overlayBackend()

	erroredBase := &irtranslator.BaseCluster{
		Cluster: &envoyclusterv3.Cluster{Name: backend.ClusterName()},
		Error:   errors.New("base boom"),
	}
	perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, context.Background(), ir.UniquelyConnectedClient{}, backend, erroredBase)
	require.NoError(t, err)
	assert.Nil(t, perClient, "errored base must short-circuit to a no-op")
	assert.Equal(t, 0, overlayCalls, "overlays must not run for an errored base")
}

// TestApplyPerClient_InlineCLAMaterializesAndIsolatesBaseEndpoints exercises the
// inline-CLA path: a STRICT_DNS backend with inline endpoints and no overlay
// must still materialize a per-client cluster (the CLA is UCC-dependent via
// PrioritizeEndpoints). It must build the LoadAssignment without mutating either
// the base cluster proto or the base EndpointInputs that a per-client endpoint
// hook writes to.
func TestApplyPerClient_InlineCLAMaterializesAndIsolatesBaseEndpoints(t *testing.T) {
	endpointGK := schema.GroupKind{Group: "test", Kind: "Endpoints"}
	bt := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "group", Kind: "kind"}: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS}
					eps := ir.NewEndpointsForBackend(in)
					eps.Add(ir.PodLocality{Region: "r1"}, ir.EndpointWithMd{LbEndpoint: pipeEndpoint("a")})
					return eps
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			endpointGK: {
				// Mimics destrule: writes PriorityInfo onto the per-client inputs.
				PerClientEditEndpoints: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, out sdk.EndpointInputsEditor) uint64 {
					out.SetPriorityInfo(&endpoints.PriorityInfo{})
					return 1
				},
			},
		},
	}
	backend := overlayBackend()
	ctx := context.Background()

	base := bt.TranslateBackendBase(ctx, backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error)
	require.True(t, base.SupportsInlineCLA, "STRICT_DNS cluster must support an inline CLA")
	require.NotNil(t, base.EndpointInputs)
	require.Nil(t, base.Cluster.GetLoadAssignment(), "base must not carry a per-client LoadAssignment")
	require.Nil(t, base.EndpointInputs.PriorityInfo, "base EndpointInputs must start without PriorityInfo")

	uccA := ir.NewUniquelyConnectedClient("a", "ns", nil, ir.PodLocality{Region: "r1"})
	uccB := ir.NewUniquelyConnectedClient("b", "ns", nil, ir.PodLocality{Region: "r2"})

	clusterA, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, uccA, backend, base)
	require.NoError(t, err)
	require.NotNil(t, clusterA, "inline-CLA backend must materialize a per-client cluster even with no overlay")
	assert.NotNil(t, clusterA.GetLoadAssignment(), "per-client cluster must carry the built LoadAssignment")
	assert.NotSame(t, base.Cluster, clusterA)

	// Base must remain pristine: neither the proto nor the EndpointInputs the
	// endpoint hook wrote to may be mutated by the overlay.
	assert.Nil(t, base.Cluster.GetLoadAssignment(), "inline-CLA build must not mutate the shared base proto")
	assert.Nil(t, base.EndpointInputs.PriorityInfo,
		"the endpoint editor must leave base EndpointInputs untouched")

	clusterB, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, uccB, backend, base)
	require.NoError(t, err)
	require.NotNil(t, clusterB)
	assert.NotSame(t, clusterA, clusterB, "each client must get an independent inline-CLA proto")
}

func TestApplyPerClient_ReevaluatesInlineCLAAfterOverlay(t *testing.T) {
	t.Run("overlay changes inline cluster to EDS", func(t *testing.T) {
		endpointCalls := 0
		bt := inlineEndpointBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
			{Group: "test", Kind: "Overlay"}: {
				PerClientClusterOverlay: func(krt.HandlerContext, context.Context, ir.UniquelyConnectedClient, ir.BackendObjectIR) *sdk.ClusterOverlay {
					return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
						out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					}}
				},
				PerClientEditEndpoints: func(krt.HandlerContext, context.Context, ir.UniquelyConnectedClient, sdk.EndpointInputsEditor) uint64 {
					endpointCalls++
					return 0
				},
			},
		}, nil)
		backend := overlayBackend()
		base := bt.TranslateBackendBase(t.Context(), backend)
		require.True(t, base.NeedsInlineCLA(), "precondition: the STRICT_DNS base needs a per-client CLA")

		perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, t.Context(), ir.UniquelyConnectedClient{}, backend, base)
		require.NoError(t, err)
		require.NotNil(t, perClient)
		assert.Equal(t, envoyclusterv3.Cluster_EDS, perClient.GetType())
		assert.Nil(t, perClient.GetLoadAssignment(), "an EDS overlay must not inherit the base's inline-CLA requirement")
		assert.Zero(t, endpointCalls, "endpoint hooks must stay lazy when the final cluster does not consume an inline CLA")
	})

	t.Run("overlay removes an existing inline load assignment", func(t *testing.T) {
		original := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "original"}
		bt := inlineEndpointBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
			{Group: "test", Kind: "Overlay"}: {
				PerClientClusterOverlay: func(krt.HandlerContext, context.Context, ir.UniquelyConnectedClient, ir.BackendObjectIR) *sdk.ClusterOverlay {
					return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
						out.LoadAssignment = nil
					}}
				},
			},
		}, original)
		backend := overlayBackend()
		base := bt.TranslateBackendBase(t.Context(), backend)
		require.False(t, base.NeedsInlineCLA(), "precondition: the base already has an inline CLA")

		perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, t.Context(), ir.UniquelyConnectedClient{}, backend, base)
		require.NoError(t, err)
		require.NotNil(t, perClient)
		require.NotNil(t, perClient.GetLoadAssignment(), "the final inline cluster must get a replacement CLA")
		assert.Equal(t, backend.ClusterName(), perClient.GetLoadAssignment().GetClusterName())
		assert.NotSame(t, original, perClient.GetLoadAssignment())
	})

	t.Run("overlay supplies its own load assignment", func(t *testing.T) {
		endpointCalls := 0
		overlayCLA := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "overlay"}
		bt := inlineEndpointBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
			{Group: "test", Kind: "Overlay"}: {
				PerClientClusterOverlay: func(krt.HandlerContext, context.Context, ir.UniquelyConnectedClient, ir.BackendObjectIR) *sdk.ClusterOverlay {
					return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
						out.LoadAssignment = overlayCLA
					}}
				},
				PerClientEditEndpoints: func(krt.HandlerContext, context.Context, ir.UniquelyConnectedClient, sdk.EndpointInputsEditor) uint64 {
					endpointCalls++
					return 0
				},
			},
		}, nil)
		backend := overlayBackend()
		base := bt.TranslateBackendBase(t.Context(), backend)
		require.True(t, base.NeedsInlineCLA())

		perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, t.Context(), ir.UniquelyConnectedClient{}, backend, base)
		require.NoError(t, err)
		require.NotNil(t, perClient)
		assert.Same(t, overlayCLA, perClient.GetLoadAssignment(), "the overlay's CLA must remain authoritative")
		assert.Zero(t, endpointCalls, "endpoint hooks must not run when the overlay already supplied the CLA")
	})
}

func TestApplyPerClient_ReappliesGatewayBackendClientCertificateAfterOverlay(t *testing.T) {
	overlayGK := schema.GroupKind{Group: "test", Kind: "Overlay"}
	bt := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		overlayGK: {
			PerClientClusterOverlay: func(krt.HandlerContext, context.Context, ir.UniquelyConnectedClient, ir.BackendObjectIR) *sdk.ClusterOverlay {
				return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
					out.TransportSocket = upstreamTLSTransportSocket(t, "overlay.example.com", "overlay-sds-secret")
				}}
			},
		},
	})
	backend := overlayBackend()
	backend.GatewayBackendClientCertificate = &ir.GatewayBackendClientCertificateIR{
		Certificate: ir.TLSCertificate{
			CertChain:  []byte("gateway-cert"),
			PrivateKey: []byte("gateway-key"),
		},
	}
	base := bt.TranslateBackendBase(t.Context(), backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error)

	perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, t.Context(), ir.UniquelyConnectedClient{}, backend, base)
	require.NoError(t, err)
	require.NotNil(t, perClient)

	tlsContext := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, perClient.GetTransportSocket().GetTypedConfig().UnmarshalTo(tlsContext))
	assert.Equal(t, "overlay.example.com", tlsContext.GetSni(), "the overlay's TLS settings must be preserved")
	require.Len(t, tlsContext.GetCommonTlsContext().GetTlsCertificates(), 1)
	assert.Equal(t, "gateway-cert", tlsContext.GetCommonTlsContext().GetTlsCertificates()[0].GetCertificateChain().GetInlineString())
	assert.Equal(t, "gateway-key", tlsContext.GetCommonTlsContext().GetTlsCertificates()[0].GetPrivateKey().GetInlineString())
	assert.Empty(t, tlsContext.GetCommonTlsContext().GetTlsCertificateSdsSecretConfigs(),
		"the resolved Gateway certificate must replace an overlay-provided SDS client identity")
}

func inlineEndpointBackendTranslator(
	policies map[schema.GroupKind]sdk.PolicyPlugin,
	loadAssignment *envoyendpointv3.ClusterLoadAssignment,
) *irtranslator.BackendTranslator {
	return &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "group", Kind: "kind"}: {
				InitEnvoyBackend: func(_ context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS}
					out.LoadAssignment = loadAssignment
					eps := ir.NewEndpointsForBackend(in)
					eps.Add(ir.PodLocality{}, ir.EndpointWithMd{LbEndpoint: pipeEndpoint("endpoint")})
					return eps
				},
			},
		},
		ContributedPolicies: policies,
	}
}

func upstreamTLSTransportSocket(t *testing.T, sni, sdsSecret string) *envoycorev3.TransportSocket {
	t.Helper()
	typedConfig, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
		Sni: sni,
		CommonTlsContext: &envoytlsv3.CommonTlsContext{
			TlsCertificateSdsSecretConfigs: []*envoytlsv3.SdsSecretConfig{{Name: sdsSecret}},
		},
	})
	require.NoError(t, err)
	return &envoycorev3.TransportSocket{
		Name:       envoywellknown.TransportSocketTls,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}
}

func TestApplyPerClient_LegacyEndpointPluginDeepCopiesNestedInputs(t *testing.T) {
	endpointGK := schema.GroupKind{Group: "test", Kind: "LegacyEndpoints"}
	locality := ir.PodLocality{Region: "r1"}
	bt := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "group", Kind: "kind"}: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS}
					eps := ir.NewEndpointsForBackend(in)
					eps.BackendLabels = map[string]string{"owner": "base"}
					eps.Add(locality, endpointWithLabels("10.0.0.1", map[string]string{"owner": "base"}))
					return eps
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			endpointGK: {
				PerClientProcessEndpoints: func(_ krt.HandlerContext, _ context.Context, ucc ir.UniquelyConnectedClient, out *sdk.EndpointsInputs) uint64 {
					if ucc.Role != "mutate" {
						return 0
					}
					out.EndpointsForBackend.BackendLabels["owner"] = "mutated"
					out.EndpointsForBackend.LbEps[locality][0].EndpointMd.Labels["owner"] = "mutated"
					out.EndpointsForBackend.LbEps[locality][0].GetEndpoint().GetAddress().GetSocketAddress().Address = "127.0.0.1"
					return 1
				},
			},
		},
	}
	backend := overlayBackend()
	base := bt.TranslateBackendBase(context.Background(), backend)
	require.NotNil(t, base)
	require.NotNil(t, base.EndpointInputs)

	mutated, err := bt.ApplyPerClient(krt.TestingDummyContext{}, context.Background(), ir.UniquelyConnectedClient{Role: "mutate"}, backend, base)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", mutated.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress())

	assert.Equal(t, "base", base.EndpointInputs.EndpointsForBackend.BackendLabels["owner"])
	assert.Equal(t, "base", base.EndpointInputs.EndpointsForBackend.LbEps[locality][0].EndpointMd.Labels["owner"])
	assert.Equal(t, "10.0.0.1", base.EndpointInputs.EndpointsForBackend.LbEps[locality][0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress())

	pristine, err := bt.ApplyPerClient(krt.TestingDummyContext{}, context.Background(), ir.UniquelyConnectedClient{Role: "pristine"}, backend, base)
	require.NoError(t, err)
	require.Equal(t, "10.0.0.1", pristine.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress())
}

// TestTranslateBackendBase_NilForUnsupportedGroupKind: a backend whose GroupKind
// has no contributed translator cannot produce even a blackhole base cluster.
func TestTranslateBackendBase_NilForUnsupportedGroupKind(t *testing.T) {
	bt := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{},
	}
	backend := overlayBackend()

	base := bt.TranslateBackendBase(context.Background(), backend)
	assert.Nil(t, base, "unsupported GroupKind must yield a nil base")
}

// edsWithConfigBackendTranslator mirrors what a real EDS backend plugin (e.g.
// kubernetes) emits: the discovery type AND an EdsClusterConfig. The latter is what
// defaultLocalityConfig gates on, so it is required to exercise that path.
func edsWithConfigBackendTranslator(policies map[schema.GroupKind]sdk.PolicyPlugin) *irtranslator.BackendTranslator {
	return &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "group", Kind: "kind"}: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					out.EdsClusterConfig = &envoyclusterv3.Cluster_EdsClusterConfig{
						EdsConfig: &envoycorev3.ConfigSource{
							ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{Ads: &envoycorev3.AggregatedConfigSource{}},
						},
					}
					return nil
				},
			},
		},
		ContributedPolicies: policies,
	}
}

// inlineRedirectOverlay mimics the waypoint ingress redirect: it converts the EDS
// cluster into a STATIC one with an inlined CLA whose LocalityLbEndpoints carry no
// load_balancing_weight — exactly the shape that cannot coexist with locality
// weighted LB.
func inlineRedirectOverlay(gk schema.GroupKind) map[schema.GroupKind]sdk.PolicyPlugin {
	return map[schema.GroupKind]sdk.PolicyPlugin{
		gk: {
			PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
				return &sdk.ClusterOverlay{
					Mutate: func(out *envoyclusterv3.Cluster) {
						out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}
						out.EdsClusterConfig = nil
						out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
							ClusterName: out.GetName(),
							Endpoints: []*envoyendpointv3.LocalityLbEndpoints{
								{LbEndpoints: []*envoyendpointv3.LbEndpoint{pipeEndpoint("redirect")}},
							},
						}
					},
				}
			},
		},
	}
}

// TestApplyPerClient_UndoesDefaultedLocalityOnInlineOverlay is the ordering
// regression guard for the base/overlay split. defaultLocalityConfig declines to
// touch plugin-provided inline clusters because their CLAs carry no per-locality
// load_balancing_weight, and Envoy rejects locality weighted LB without it. Before
// the split, per-client hooks ran first, so that guard saw the final cluster. Now the
// guard runs on the still-EDS base, so an overlay that inlines the CLA afterwards
// must undo the default rather than ship a cluster Envoy will reject.
func TestApplyPerClient_UndoesDefaultedLocalityOnInlineOverlay(t *testing.T) {
	overlayGK := schema.GroupKind{Group: "test", Kind: "Overlay"}
	bt := edsWithConfigBackendTranslator(inlineRedirectOverlay(overlayGK))
	backend := overlayBackend()
	ctx := context.Background()

	base := bt.TranslateBackendBase(ctx, backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error)
	require.True(t, base.DefaultedLocalityConfig, "an EDS base with no LB policy must get the locality default")
	require.NotNil(t, base.Cluster.GetCommonLbConfig().GetLocalityWeightedLbConfig(),
		"precondition: the base carries the defaulted locality mode")

	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, ucc, backend, base)
	require.NoError(t, err)
	require.NotNil(t, perClient, "the overlay applies, so a per-client cluster must materialize")

	require.NotNil(t, perClient.GetLoadAssignment(), "precondition: the overlay inlined a CLA")
	require.Nil(t, perClient.GetEdsClusterConfig(), "precondition: the overlay dropped EDS")
	assert.Nil(t, perClient.GetCommonLbConfig().GetLocalityConfigSpecifier(),
		"locality weighting must not survive onto an inlined CLA with no load_balancing_weight")
	assert.Nil(t, perClient.GetCommonLbConfig(),
		"CommonLbConfig was allocated only to hold the default, so it must not be emitted empty")

	assert.NotNil(t, base.Cluster.GetCommonLbConfig().GetLocalityWeightedLbConfig(),
		"undoing the default must not reach back into the shared base")
}

// TestApplyPerClient_KeepsDefaultedLocalityWhenStillEDS is the other half: an
// overlay that leaves the cluster EDS keeps the defaulted locality mode, so the undo
// above cannot regress the ordinary per-client path.
func TestApplyPerClient_KeepsDefaultedLocalityWhenStillEDS(t *testing.T) {
	overlayGK := schema.GroupKind{Group: "test", Kind: "Overlay"}
	bt := edsWithConfigBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		overlayGK: {
			PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
				return &sdk.ClusterOverlay{
					Mutate: func(out *envoyclusterv3.Cluster) {
						out.OutlierDetection = &envoyclusterv3.OutlierDetection{}
					},
				}
			},
		},
	})
	backend := overlayBackend()
	ctx := context.Background()

	base := bt.TranslateBackendBase(ctx, backend)
	require.NotNil(t, base)
	require.True(t, base.DefaultedLocalityConfig)

	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, ucc, backend, base)
	require.NoError(t, err)
	require.NotNil(t, perClient)

	assert.NotNil(t, perClient.GetCommonLbConfig().GetLocalityWeightedLbConfig(),
		"a cluster that is still EDS must keep the defaulted locality mode")
}

// TestApplyPerClient_LeavesOverlayChosenLocalityMode: the undo is scoped to the mode
// defaultLocalityConfig itself installed. An overlay that inlines the CLA and picks
// its own locality mode has made a deliberate choice, which must survive.
func TestApplyPerClient_LeavesOverlayChosenLocalityMode(t *testing.T) {
	overlayGK := schema.GroupKind{Group: "test", Kind: "Overlay"}
	bt := edsWithConfigBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		overlayGK: {
			PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
				return &sdk.ClusterOverlay{
					Mutate: func(out *envoyclusterv3.Cluster) {
						out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}
						out.EdsClusterConfig = nil
						out.CommonLbConfig = &envoyclusterv3.Cluster_CommonLbConfig{
							LocalityConfigSpecifier: &envoyclusterv3.Cluster_CommonLbConfig_ZoneAwareLbConfig_{
								ZoneAwareLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ZoneAwareLbConfig{},
							},
						}
					},
				}
			},
		},
	})
	backend := overlayBackend()
	ctx := context.Background()

	base := bt.TranslateBackendBase(ctx, backend)
	require.NotNil(t, base)
	require.True(t, base.DefaultedLocalityConfig)

	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	perClient, err := bt.ApplyPerClient(krt.TestingDummyContext{}, ctx, ucc, backend, base)
	require.NoError(t, err)
	require.NotNil(t, perClient)

	assert.NotNil(t, perClient.GetCommonLbConfig().GetZoneAwareLbConfig(),
		"an overlay's own locality choice must not be undone")
}

func pipeEndpoint(path string) *envoyendpointv3.LbEndpoint {
	return &envoyendpointv3.LbEndpoint{
		HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
			Endpoint: &envoyendpointv3.Endpoint{
				Address: &envoycorev3.Address{
					Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: path}},
				},
			},
		},
	}
}
