package irtranslator

// White-box benchmark (internal package) comparing the per-client cluster fan-out before and after
// the base+overlay refactor.
//
//	Old: full translation per (backend, client) pair, in place — no base cache, no clone, no fast
//	     path. This faithfully reconstructs the former monolithic translation body.
//	New: base translated once per backend (TranslateBackendBase), then a cheap per-client overlay
//	     (ApplyPerClient) per pair. When no overlay applies, ApplyPerClient returns nil and the
//	     caller shares the base proto — the dominant path for a sparsely-matched destination rule.
//
// Swept across Istio off/on (whether a per-client overlay plugin is registered) and light/heavy
// backends (how expensive the client-independent base translation is). The delta scales with the
// number of (backend, client) pairs and with how sparsely the per-client overlay actually matches.
//
// Run with:
//
//	go test -run=^$ -bench=BenchmarkPerClientClusters -benchmem \
//	    ./pkg/kgateway/translator/irtranslator/

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	benchNumBackends = 200
	benchNumClients  = 20
)

var benchGK = schema.GroupKind{Group: "bench", Kind: "Backend"}

// benchSink prevents the compiler from optimizing away the translated clusters.
var benchSink *envoyclusterv3.Cluster

func benchInit(heavy bool) func(context.Context, ir.BackendObjectIR, *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
	return func(_ context.Context, _ ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
		out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
		out.EdsClusterConfig = &envoyclusterv3.Cluster_EdsClusterConfig{
			EdsConfig: &envoycorev3.ConfigSource{
				ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{Ads: &envoycorev3.AggregatedConfigSource{}},
			},
		}
		if heavy {
			// Simulate a TLS backend: a proto marshal as part of base translation.
			if tlsAny, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{Sni: out.GetName()}); err == nil {
				out.TransportSocket = &envoycorev3.TransportSocket{
					Name:       "envoy.transport_sockets.tls",
					ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: tlsAny},
				}
			}
		}
		return nil
	}
}

func benchTranslator(istioOn, heavy bool) *BackendTranslator {
	t := &BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			benchGK: {InitEnvoyBackend: benchInit(heavy)},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{},
		// Mode left as zero value (non-strict): validation is skipped, matching the default deployment.
	}
	if istioOn {
		// A destination-rule-like per-client overlay: it only mutates clients whose label matches
		// (a "destination rule" is present) and self-gates by returning nil for everyone else, so
		// the translator can skip the per-client clone entirely for non-matching pairs.
		t.ContributedPolicies[benchGK] = sdk.PolicyPlugin{
			PerClientClusterOverlay: func(_ krt.HandlerContext, _ context.Context, ucc ir.UniquelyConnectedClient, _ ir.BackendObjectIR) *sdk.ClusterOverlay {
				if ucc.Labels["bench-mutate"] != "yes" {
					return nil
				}
				return &sdk.ClusterOverlay{
					Mutate: func(out *envoyclusterv3.Cluster) {
						out.OutlierDetection = &envoyclusterv3.OutlierDetection{}
					},
				}
			},
		}
	}
	return t
}

func benchBackends(n int) []*ir.BackendObjectIR {
	out := make([]*ir.BackendObjectIR, n)
	for i := range n {
		b := ir.NewBackendObjectIR(ir.ObjectSource{
			Group:     "bench",
			Kind:      "Backend",
			Namespace: "default",
			Name:      fmt.Sprintf("b%d", i),
		}, 8080, "", "")
		out[i] = &b
	}
	return out
}

func benchUCCs(m int) []ir.UniquelyConnectedClient {
	out := make([]ir.UniquelyConnectedClient, m)
	for i := range m {
		// Model sparse destination-rule matching: only ~1 in 10 clients is targeted by a DR.
		mutate := "no"
		if i%10 == 0 {
			mutate = "yes"
		}
		out[i] = ir.NewUniquelyConnectedClient(
			fmt.Sprintf("role%d", i),
			"default",
			map[string]string{"bench-mutate": mutate, "idx": strconv.Itoa(i)},
			ir.PodLocality{Region: "r", Zone: fmt.Sprintf("z%d", i%3)},
		)
	}
	return out
}

// legacyTranslate reconstructs the pre-refactor per-pair translation: a full, in-place translation
// for every (backend, client), with no base caching, no clone, and no fast path. The benchmark uses
// EDS backends (benchInit returns nil inline endpoints), so the inline-CLA branch is intentionally
// omitted here — it would be dead code for this input and is identically absent on the New path.
func (t *BackendTranslator) legacyTranslate(ctx context.Context, kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient, backend *ir.BackendObjectIR) *envoyclusterv3.Cluster {
	process := t.ContributedBackends[backend.GetGroupKind()]
	out := initializeCluster(backend)
	process.InitEnvoyBackend(ctx, *backend, out)
	processDnsLookupFamily(out, t.CommonCols)
	_ = t.applyBasePolicies(ctx, backend, out)
	// Per-client overlays applied in place, once per pair — the pre-refactor behavior.
	for _, policyPlugin := range t.ContributedPolicies {
		if policyPlugin.PerClientClusterOverlay == nil {
			continue
		}
		if ov := policyPlugin.PerClientClusterOverlay(kctx, ctx, ucc, *backend); ov != nil && ov.Mutate != nil {
			ov.Mutate(out)
		}
	}
	_ = applyGatewayBackendClientCertificate(out, backend)
	return out
}

func benchmarkPerClientClusters(b *testing.B, istioOn, heavy bool) {
	t := benchTranslator(istioOn, heavy)
	backends := benchBackends(benchNumBackends)
	uccs := benchUCCs(benchNumClients)
	ctx := context.Background()
	var kctx krt.TestingDummyContext

	b.Run("Old", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			for _, backend := range backends {
				for _, ucc := range uccs {
					benchSink = t.legacyTranslate(ctx, kctx, ucc, backend)
				}
			}
		}
	})

	b.Run("New", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			bases := make([]*BaseCluster, len(backends))
			for j, backend := range backends {
				bases[j] = t.TranslateBackendBase(ctx, backend)
			}
			for j, backend := range backends {
				for _, ucc := range uccs {
					c, _ := t.ApplyPerClient(kctx, ctx, ucc, backend, bases[j])
					if c == nil {
						// Fast path: no per-client variation, the base proto is shared.
						c = bases[j].Cluster
					}
					benchSink = c
				}
			}
		}
	})
}

func BenchmarkPerClientClusters(b *testing.B) {
	for _, istioOn := range []bool{false, true} {
		for _, heavy := range []bool{false, true} {
			b.Run(fmt.Sprintf("istio=%v/heavy=%v", istioOn, heavy), func(b *testing.B) {
				benchmarkPerClientClusters(b, istioOn, heavy)
			})
		}
	}
}
