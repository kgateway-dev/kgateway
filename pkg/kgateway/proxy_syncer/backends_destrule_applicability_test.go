package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/require"
	"istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/destrule"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// TestBaseRetranslatesWhenARuleForItsHostAppears pins the contract behind
// PerClientEndpointsMayApply taking the base translation's HandlerContext: the
// predicate's fetch registers a KRT dependency, so a base whose inline CLA was
// built once (no rule named its host) is re-translated when a rule appears and
// moves to the per-client CLA path, and moves back when the rule is removed.
// The unit tests for HasRulesForHost use a static collection and prove the
// answer; this proves the invalidation, through the real base and per-client
// collections and the real DestinationRule index.
func TestBaseRetranslatesWhenARuleForItsHostAppears(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	const host = "svc.ns.svc.cluster.local"
	rules := krt.NewStaticCollection[destrule.DestinationRuleWrapper](nil, nil, krtopts.ToOptions("DestinationRules")...)
	index := destrule.NewDestRuleIndexFromCollection(rules)

	// An endpoint hook shaped like the destrule plugin's: it may apply only to
	// backends whose host some rule names, and when it runs it orders endpoints
	// by the client's locality.
	policyGK := schema.GroupKind{Group: "networking.istio.io", Kind: "DestinationRule"}
	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "", Kind: "Service"}: {
				InitEnvoyBackend: func(_ context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS}
					eps := ir.NewEndpointsForBackend(in)
					eps.Add(ir.PodLocality{Region: "r1", Zone: "z1"}, ir.EndpointWithMd{LbEndpoint: lbEndpointPipe("z1")})
					eps.Add(ir.PodLocality{Region: "r1", Zone: "z2"}, ir.EndpointWithMd{LbEndpoint: lbEndpointPipe("z2")})
					return eps
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			policyGK: {
				PerClientEndpointsMayApply: func(kctx krt.HandlerContext, in ir.BackendObjectIR) bool {
					return index.HasRulesForHost(kctx, in.CanonicalHostname)
				},
				PerClientEditEndpoints: func(kctx krt.HandlerContext, _ context.Context, _ ir.UniquelyConnectedClient, out sdk.EndpointInputsEditor) uint64 {
					if !index.HasRulesForHost(kctx, out.Hostname()) {
						return 0
					}
					out.SetPriorityInfo(&endpoints.PriorityInfo{})
					return 1
				},
			},
		},
	}

	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: "", Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")
	backend.CanonicalHostname = host
	backends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{&backend}, krtopts.ToOptions("FinalBackends")...)
	client := ir.NewUniquelyConnectedClient("c", "ns", nil, ir.PodLocality{Region: "r1", Zone: "z1"})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{client}, krtopts.ToOptions("UCCs")...)

	pcc := NewPerClientEnvoyClusters(ctx, krtopts, translator, backends, uccs)
	name := backend.ClusterName()

	// sharedOnBase reports whether the client is served the base proto itself,
	// with the CLA built on the base.
	sharedOnBase := func() bool {
		baseRow := pcc.base.GetKey(name)
		stored := storedClustersForClient(pcc, client)[name]
		return baseRow != nil && stored != nil &&
			baseRow.Cluster.Is(stored) && stored.GetLoadAssignment() != nil
	}
	// perClientCLA reports whether the base is CLA-less and the client holds its
	// own cluster carrying a per-client CLA.
	perClientCLA := func() bool {
		baseRow := pcc.base.GetKey(name)
		stored := storedClustersForClient(pcc, client)[name]
		return baseRow != nil && stored != nil &&
			baseRow.Cluster.BorrowForRead().GetLoadAssignment() == nil &&
			!baseRow.Cluster.Is(stored) && stored.GetLoadAssignment() != nil
	}

	require.Eventually(t, sharedOnBase, 5*time.Second, 10*time.Millisecond,
		"with no rule for the host, the CLA must be built once on the base and shared")

	// A rule for the host appears. The base must re-translate: its fetch through
	// the index registered the dependency, and the answer changed.
	rule := destrule.DestinationRuleWrapper{DestinationRule: &networkingclient.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "rule"},
		Spec:       v1alpha3.DestinationRule{Host: host},
	}}
	rules.UpdateObject(rule)
	require.Eventually(t, perClientCLA, 5*time.Second, 10*time.Millisecond,
		"a rule naming the host must move the backend to the per-client CLA path")

	// A rule for another host changes nothing: the dependency is keyed by host.
	other := destrule.DestinationRuleWrapper{DestinationRule: &networkingclient.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "other"},
		Spec:       v1alpha3.DestinationRule{Host: "other.ns.svc.cluster.local"},
	}}
	rules.UpdateObject(other)
	time.Sleep(50 * time.Millisecond)
	require.True(t, perClientCLA(), "a rule for another host must not change the path")

	// The rule for the host goes away: the base re-translates again and the CLA
	// returns to the shared base.
	rules.DeleteObject(rule.ResourceName())
	require.Eventually(t, sharedOnBase, 5*time.Second, 10*time.Millisecond,
		"removing the last rule for the host must move the CLA back onto the shared base")
}
