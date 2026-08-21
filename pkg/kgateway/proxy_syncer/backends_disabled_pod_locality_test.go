package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestSparseClustersTrackSharedClientCapabilityWithoutGlobalWithholding(t *testing.T) {
	t.Setenv("KGW_XDS_FIRST_CONNECT_DELAY", "0")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	callbacks, buildClients := krtcollections.NewUniquelyConnectedClients(nil, false)
	var pods krt.Collection[krtcollections.LocalityPod]
	clients := buildClients(ctx, krtopts, pods)
	clients.WaitUntilSynced(ctx.Done())

	backendGK := schema.GroupKind{Kind: "Service"}
	overlayGK := schema.GroupKind{Group: "example.io", Kind: "LocalClusterCapability"}
	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(_ context.Context, _ ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		ContributedPolicies: sdk.ContributesPolicies{
			overlayGK: {
				PerClientClusterOverlay: func(_ krt.HandlerContext, _ context.Context, ucc ir.UniquelyConnectedClient, _ ir.BackendObjectIR) *sdk.ClusterOverlay {
					if !ucc.KnowsLocalCluster {
						return nil
					}
					return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
						out.AltStatName = "local-cluster-capable"
					}}
				},
			},
		},
	}
	backend := clustersTestBackend("backend")
	backends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{backend}, krtopts.ToOptions("DisabledPodLocalityBackends")...)
	clusters := NewPerClientEnvoyClusters(ctx, krtopts, translator, backends, clients)

	role := wellknown.GatewayApiProxyValue + "~ns~gw"
	node := func(id string) *envoycorev3.Node {
		return &envoycorev3.Node{
			Id: id,
			Metadata: &structpb.Struct{Fields: map[string]*structpb.Value{
				xds.RoleKey: structpb.NewStringValue(role),
			}},
		}
	}
	request := func(streamID int64, typeURL string, resources ...string) {
		require.NoError(t, callbacks.OnStreamRequest(streamID, &envoy_service_discovery_v3.DiscoveryRequest{
			Node:          node(role),
			TypeUrl:       typeURL,
			ResourceNames: resources,
		}))
	}
	assertState := func(knows bool, altStatName string) {
		require.Eventually(t, func() bool {
			current := clients.List()
			if len(current) != 1 || current[0].KnowsLocalCluster != knows {
				return false
			}
			resolved := clusters.FetchClustersForClient(krt.TestingDummyContext{}, current[0])
			return len(resolved) == 1 && resolved[0].Cluster.Clone().GetAltStatName() == altStatName
		}, 5*time.Second, 10*time.Millisecond)
	}

	request(1, resource.EndpointType, "gw.ns")
	assertState(true, "local-cluster-capable")

	// An unconfirmed sibling changes the shared UCC in place. Sparse absence
	// must be resolved against that exact client version, without withholding
	// an unrelated or stale delta indefinitely.
	request(2, resource.ClusterType)
	assertState(false, "")

	request(2, resource.EndpointType, "gw.ns")
	assertState(true, "local-cluster-capable")

	request(3, resource.ClusterType)
	assertState(false, "")
	callbacks.OnStreamClosed(3, nil)
	assertState(true, "local-cluster-capable")
}
