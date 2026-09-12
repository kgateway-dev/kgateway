package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// Every consumer assumes the cluster is named after the backend's memoized
// ClusterName: routes reference it, status is keyed on it, the EDS pipeline
// names its CLA after it. Nothing in tree renames it, so this pins the
// containment: a backend whose translation renamed the cluster is recorded as
// errored under the expected name, alone, and the clients keep every other
// backend. It must not simply vanish: an absent row means no errored record,
// so its CLA would stay in EDS unfiltered and no status would be written.
func TestNewPerClientEnvoyClusters_RenamedClusterIsErroredNotDropped(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	backendGK := schema.GroupKind{Group: "group", Kind: "kind"}

	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					if in.GetName() == "renamed" {
						out.Name = "something-else"
					}
					return nil
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{},
	}

	good := ir.NewBackendObjectIR(ir.ObjectSource{Group: "group", Kind: "kind", Namespace: "ns", Name: "good"}, 80, "", "")
	renamed := ir.NewBackendObjectIR(ir.ObjectSource{Group: "group", Kind: "kind", Namespace: "ns", Name: "renamed"}, 80, "", "")
	finalBackends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{&good, &renamed},
		krtopts.ToOptions("FinalBackends")...)
	client := ir.NewUniquelyConnectedClient("c", "ns", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{client}, krtopts.ToOptions("UCCs")...)

	pcc := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)
	require.Eventually(t, pcc.HasSynced, time.Second, 10*time.Millisecond)

	var got []string
	require.Eventually(t, func() bool {
		got = storedClusterNamesForClient(pcc, client)
		return len(got) > 0
	}, 2*time.Second, 20*time.Millisecond,
		"a renamed cluster must not withhold the client's CDS")

	require.Len(t, got, 1, "only the renamed backend should be excluded from the payload")
	assert.Equal(t, good.ClusterName(), got[0])

	// The renamed backend is not gone: it is an errored base row under the name
	// every consumer expects, so its CLA is filtered and status can report it.
	baseRow := pcc.base.GetKey(renamed.ClusterName())
	require.NotNil(t, baseRow, "the renamed backend must keep a base row under its expected cluster name")
	require.Error(t, baseRow.Error, "the base row must carry the rename as its translation error")
	assert.Contains(t, baseRow.Error.Error(), "something-else")
	statusRow := pcc.StatusClusters().GetKey("/" + renamed.ClusterName())
	require.NotNil(t, statusRow, "status must see the renamed backend's error")
	assert.Equal(t, renamed.GetObjectSource(), statusRow.BackendSource)
	perClientRow := pcc.perClient.GetKey(client.ResourceName())
	require.NotNil(t, perClientRow)
	assert.Contains(t, perClientRow.erroredClusters, renamed.ClusterName(),
		"the client's payload must track the renamed backend as errored so its CLA is filtered")
}
