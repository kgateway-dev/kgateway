package destrule

import (
	"context"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/overlaytest"
)

const drHost = "reviews.default.svc.cluster.local"

func newDestrulePlugin(t *testing.T, drs ...DestinationRuleWrapper) *destrulePlugin {
	t.Helper()
	col := krt.NewStaticCollection(nil, drs)
	return &destrulePlugin{
		destinationRulesIndex: DestinationRuleIndex{
			Destrules:  col,
			ByHostname: newDestruleIndex(col),
		},
	}
}

func destRule(name string, tp *v1alpha3.TrafficPolicy) DestinationRuleWrapper {
	return DestinationRuleWrapper{
		DestinationRule: &networkingclient.DestinationRule{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
			Spec: v1alpha3.DestinationRule{
				Host:          drHost,
				TrafficPolicy: tp,
			},
		},
	}
}

func drBackend() ir.BackendObjectIR {
	b := ir.NewBackendObjectIR(ir.ObjectSource{Group: "", Kind: "Service", Namespace: "ns", Name: "reviews"}, 80, "", "")
	b.CanonicalHostname = drHost
	return b
}

// TestClusterOverlay_NilWhenNoDestinationRule: the self-gating overlay must
// return nil when no destination rule matches the backend, so no per-client
// delta is materialized. This is the dominant path the sparseness relies on.
func TestClusterOverlay_NilWhenNoDestinationRule(t *testing.T) {
	d := newDestrulePlugin(t) // no destination rules at all
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})

	got := d.clusterOverlay(krt.TestingDummyContext{}, context.Background(), ucc, drBackend())
	assert.Nil(t, got, "no matching destination rule must yield no overlay")
}

// TestClusterOverlay_NilWhenNoOutlierDetection: a matching destination rule with
// no outlier detection must still return nil. Outlier detection gates every
// cluster-level change the plugin makes (outlier detection itself, locality LB
// config, and TCP keepalive are all applied only when it is present, matching the
// pre-overlay processBackend), so a DR without it contributes nothing per client.
func TestClusterOverlay_NilWhenNoOutlierDetection(t *testing.T) {
	d := newDestrulePlugin(t, destRule("dr", &v1alpha3.TrafficPolicy{
		// Locality settings alone reach the cluster only alongside outlier detection;
		// on their own they affect endpoint prioritization, not the cluster overlay.
		LoadBalancer: &v1alpha3.LoadBalancerSettings{
			LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{},
		},
	}))
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})

	got := d.clusterOverlay(krt.TestingDummyContext{}, context.Background(), ucc, drBackend())
	assert.Nil(t, got, "destination rule without outlier detection must yield no cluster overlay")
}

// TestClusterOverlay_AppliesOutlierDetection: a matching destination rule with
// outlier detection must return an overlay that writes OutlierDetection onto the
// cluster. This confirms the positive path and that Mutate produces the expected
// mutation.
func TestClusterOverlay_AppliesOutlierDetection(t *testing.T) {
	d := newDestrulePlugin(t, destRule("dr", &v1alpha3.TrafficPolicy{
		OutlierDetection: &v1alpha3.OutlierDetection{
			Consecutive_5XxErrors: &wrapperspb.UInt32Value{Value: 5},
		},
	}))
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})

	got := d.clusterOverlay(krt.TestingDummyContext{}, context.Background(), ucc, drBackend())
	require.NotNil(t, got, "matching destination rule with outlier detection must produce an overlay")
	require.NotNil(t, got.Mutate)

	out := &envoyclusterv3.Cluster{Name: "reviews"}
	got.Mutate(out)
	assert.NotNil(t, out.GetOutlierDetection(), "overlay must apply outlier detection to the cluster")
	assert.Equal(t, uint32(5), out.GetOutlierDetection().GetConsecutive_5Xx().GetValue())
}

// TestOverlayInputsHash_CoversClusterOverlayInputs checks the declaration the
// plugin registers beside its overlay: every backend field whose change moves
// the overlay's output must move the hash. The rule chosen here has a top-level
// outlier policy and a different one for port 80, so both the hostname (which
// selects the rule) and the port (which selects the policy within it) change
// the output. Labels, annotations and the resourceVersion are the controls: the
// overlay does not read them, and the harness only requires that what changes
// the output is declared.
func TestOverlayInputsHash_CoversClusterOverlayInputs(t *testing.T) {
	topLevel := &v1alpha3.OutlierDetection{Consecutive_5XxErrors: &wrapperspb.UInt32Value{Value: 3}}
	portLevel := &v1alpha3.OutlierDetection{Consecutive_5XxErrors: &wrapperspb.UInt32Value{Value: 7}}
	d := newDestrulePlugin(t, destRule("rule", &v1alpha3.TrafficPolicy{
		OutlierDetection: topLevel,
		PortLevelSettings: []*v1alpha3.TrafficPolicy_PortTrafficPolicy{{
			Port:             &v1alpha3.PortSelector{Number: 80},
			OutlierDetection: portLevel,
		}},
	}))

	backend := drBackend()
	backend.Obj = &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns", Name: "reviews", UID: "uid", ResourceVersion: "1",
		Labels: map[string]string{"app": "reviews"},
	}}

	overlaytest.AssertInputsHashCoversOverlay(t, overlaytest.Case{
		Plugin:  d.policyPlugin(),
		Backend: backend,
		Clients: []ir.UniquelyConnectedClient{
			ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{}),
			ir.NewUniquelyConnectedClient("role", "other-ns", nil, ir.PodLocality{}),
		},
		Mutations: []overlaytest.Mutation{
			{Name: "hostname without a rule", Apply: func(b *ir.BackendObjectIR) {
				b.CanonicalHostname = "ratings.default.svc.cluster.local"
			}},
			{Name: "port without a port-level policy", Apply: func(b *ir.BackendObjectIR) {
				rebuilt := ir.NewBackendObjectIR(b.GetObjectSource(), 8080, "", "")
				rebuilt.CanonicalHostname = b.CanonicalHostname
				rebuilt.Obj = b.Obj
				*b = rebuilt
			}},
			{Name: "app protocol", Apply: func(b *ir.BackendObjectIR) { b.AppProtocol = ir.HTTP2AppProtocol }},
			overlaytest.SetLabel(t, "app", "other"),
			overlaytest.SetAnnotation(t, "meta.helm.sh/release-name", "x"),
			overlaytest.SetResourceVersion(t, "2"),
		},
	})
}
