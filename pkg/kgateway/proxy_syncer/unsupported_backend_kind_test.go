package proxy_syncer

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// TestUnsupportedBackendTranslationIsRecordedAsErrored goes through the real
// base and per-client collections. A backend whose group/kind has no contributed translator, or
// whose contributed BackendInit has no InitEnvoyBackend, used to yield no base
// row at all: TranslateBackendBase returned nil and the base transform dropped
// it. The backend then appeared in neither the CDS payload nor the errored set,
// so its ClusterLoadAssignment was never filtered from EDS and no Backend
// status was written.
//
// The built-in backend plugins all set InitEnvoyBackend, so this fixture uses a
// synthetic plugin registration to reach the path. It asserts the repaired
// shape: every backend has a base row, the unsupported ones carry an error
// with status attribution, the client's payload tracks them as errored, and
// the errored-cluster filter removes their CLAs from the EDS payload.
func TestUnsupportedBackendTranslationIsRecordedAsErrored(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	serviceGK := schema.GroupKind{Group: "", Kind: "Service"}
	noInitGK := schema.GroupKind{Group: "example.test", Kind: "NoInitBackend"}
	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			serviceGK: {
				InitEnvoyBackend: func(_ context.Context, _ ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
			// Contributed backends collection, no cluster initializer.
			noInitGK: {},
		},
	}

	makeBackend := func(gk schema.GroupKind, name string, errs ...error) *ir.BackendObjectIR {
		b := ir.NewBackendObjectIR(ir.ObjectSource{Group: gk.Group, Kind: gk.Kind, Namespace: "default", Name: name}, 443, "", "")
		b.Errors = errs
		return &b
	}
	healthy := makeBackend(serviceGK, "healthy")
	invalid := makeBackend(serviceGK, "invalid", errors.New("synthetic pre-existing translation error"))
	noInit := makeBackend(noInitGK, "no-init")
	unregistered := makeBackend(schema.GroupKind{Group: "example.test", Kind: "Unregistered"}, "unregistered")
	all := []*ir.BackendObjectIR{healthy, invalid, noInit, unregistered}

	ucc := ir.NewUniquelyConnectedClient("role-unsupported", "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc}, krtopts.ToOptions("UniqueClients")...)
	finalBackends := krt.NewStaticCollection(nil, all, krtopts.ToOptions("FinalBackends")...)
	pcc := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)

	// Every backend produces a base row. Before the fix only two did.
	require.Eventuallyf(t, func() bool {
		return pcc.HasSynced() && len(krt.Fetch(krt.TestingDummyContext{}, pcc.base)) == len(all)
	}, 5*time.Second, 10*time.Millisecond, "expected a base row per backend")
	var row *clustersWithErrors
	require.Eventually(t, func() bool {
		row = pcc.perClient.GetKey(ucc.ResourceName())
		return row != nil && len(row.clusters.Items) == 1
	}, 5*time.Second, 10*time.Millisecond, "the client's payload must contain exactly the healthy cluster")

	require.Contains(t, row.clusters.Items, healthy.ClusterName())
	require.NoError(t, pcc.base.GetKey(healthy.ClusterName()).Error)
	require.Error(t, pcc.base.GetKey(invalid.ClusterName()).Error, "pre-existing backend errors still produce a named errored row")

	for _, unsupported := range []*ir.BackendObjectIR{noInit, unregistered} {
		base := pcc.base.GetKey(unsupported.ClusterName())
		require.NotNil(t, base, "%s must have a base row instead of being dropped", unsupported.ClusterName())
		require.Error(t, base.Error, "the unsupported backend's row must carry its translation error")
		require.Equal(t, unsupported.GetObjectSource(), base.BackendSource, "status attribution needs the source Backend")
		require.Equal(t, envoyclusterv3.Cluster_STATIC, base.Cluster.Clone().GetType(), "errored rows carry the blackhole cluster")

		status := pcc.StatusClusters().GetKey("/" + unsupported.ClusterName())
		require.NotNil(t, status, "status must see the unsupported backend's error")
		require.Error(t, status.Error)
		require.Equal(t, unsupported.GetObjectSource(), status.BackendSource)
	}

	// The client's payload tracks every errored backend by name, which is what
	// filters their CLAs out of EDS. An unsupported backend now takes that path
	// instead of leaving an orphan CLA behind, which go-control-plane's superset
	// check would otherwise hold the client's whole EDS response on.
	errored := slices.Clone(row.erroredClusters)
	slices.Sort(errored)
	want := []string{invalid.ClusterName(), noInit.ClusterName(), unregistered.ClusterName()}
	slices.Sort(want)
	require.Equal(t, want, errored)

	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: healthy.ClusterName()}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: noInit.ClusterName()}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: unregistered.ClusterName()}},
	})
	filtered, _ := filterEndpointResourcesForClusters(row.clusters, endpoints)
	require.Len(t, filtered.Items, 1, "CLAs of unsupported backends must be filtered with the other errored clusters")
	require.Contains(t, filtered.Items, healthy.ClusterName())
	require.NotEqual(t, endpoints.Version, filtered.Version, "filtering must move the EDS version so the client is pushed the narrowed set")
}
