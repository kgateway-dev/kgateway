package proxy_syncer

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestDenseClustersTranslateBaseOnceAndRetainPerRowOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	var baseTranslations atomic.Int32
	backendGK := schema.GroupKind{Kind: "Service"}
	overlayGK := schema.GroupKind{Group: "example.io", Kind: "Overlay"}
	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(_ context.Context, _ ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					baseTranslations.Add(1)
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		ContributedPolicies: sdk.ContributesPolicies{
			overlayGK: {
				PerClientClusterOverlay: func(_ krt.HandlerContext, _ context.Context, ucc ir.UniquelyConnectedClient, _ ir.BackendObjectIR) *sdk.ClusterOverlay {
					if ucc.Role != "role-b" {
						return nil
					}
					return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
						out.AltStatName = "client-b"
					}}
				},
			},
		},
	}

	a := ir.NewUniquelyConnectedClient("role-a", "default", nil, ir.PodLocality{})
	b := ir.NewUniquelyConnectedClient("role-b", "default", nil, ir.PodLocality{})
	backend := clustersTestBackend("backend")
	clients := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{a, b}, krtopts.ToOptions("DenseTestClients")...)
	backends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{backend}, krtopts.ToOptions("DenseTestBackends")...)
	clusters := NewPerClientEnvoyClusters(ctx, krtopts, translator, backends, clients)

	require.Eventually(t, func() bool {
		return len(clusters.FetchClustersForClient(krt.TestingDummyContext{}, a)) == 1 &&
			len(clusters.FetchClustersForClient(krt.TestingDummyContext{}, b)) == 1
	}, 5*time.Second, 10*time.Millisecond)

	clusterA := clusters.FetchClustersForClient(krt.TestingDummyContext{}, a)[0].Cluster
	clusterB := clusters.FetchClustersForClient(krt.TestingDummyContext{}, b)[0].Cluster
	require.EqualValues(t, 1, baseTranslations.Load(), "base translation should run once for both clients")
	require.NotSame(t, clusterA, clusterB, "dense KRT rows must retain independent proto ownership")
	require.Empty(t, clusterA.GetAltStatName())
	require.Equal(t, "client-b", clusterB.GetAltStatName())
}
