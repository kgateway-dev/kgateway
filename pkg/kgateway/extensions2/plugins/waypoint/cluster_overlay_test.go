package waypoint

import (
	"context"
	"hash/fnv"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/require"
	istioannot "istio.io/api/annotation"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/overlaytest"
)

// TestClusterOverlay_NilForNonAmbientClient pins the cheap UCC-only fast path:
// a client without the ambient-redirection annotation can never be affected by
// the waypoint cluster overlay, so clusterOverlay must return nil before doing
// any of the expensive Gateway/service lookups. This is the gate that keeps the
// per-client cluster collection sparse for the dominant (non-ambient) client.
func TestClusterOverlay_NilForNonAmbientClient(t *testing.T) {
	// commonCols is intentionally left nil: a correct fast path returns before
	// dereferencing it, so a panic here would mean the gate regressed.
	p := &PerClientProcessor{}
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: "", Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")

	cases := map[string]map[string]string{
		"no ambient label":      {"some": "label"},
		"ambient label not set": nil,
		"ambient label != enabled": {
			istioannot.AmbientRedirection.Name: "disabled",
		},
	}
	for name, labels := range cases {
		t.Run(name, func(t *testing.T) {
			ucc := ir.NewUniquelyConnectedClient("role", "ns", labels, ir.PodLocality{})
			got := p.clusterOverlay(krt.TestingDummyContext{}, context.Background(), ucc, backend)
			if got != nil {
				t.Fatalf("expected nil overlay for non-ambient client, got %#v", got)
			}
		})
	}
}

// fakeWaypointQueries answers GetServiceWaypoint for one service key and
// nothing else. The overlay under test calls only that method.
type fakeWaypointQueries struct {
	waypointquery.WaypointQueries
	attached map[string]types.NamespacedName
}

func (f fakeWaypointQueries) GetServiceWaypoint(_ krt.HandlerContext, _ context.Context, obj metav1.Object) *types.NamespacedName {
	wp, ok := f.attached[waypointquery.ServiceKeyFromObject(obj)]
	if !ok {
		return nil
	}
	return &wp
}

// TestOverlayInputsHash_CoversClusterOverlayInputs checks the declaration the
// plugin registers beside its overlay: every backend field whose change moves
// the overlay's output must move the hash. The fixture is a Service the overlay
// applies to for an ambient client behind a non-waypoint Gateway. The
// ingress-use-waypoint label, the namespace (whose own label is consulted when
// the object's is absent), the name (which keys the waypoint lookup), the
// clusterIPs (inlined into the STATIC cluster) and the port all change the
// output. The clusterIPs are the case that motivated the declaration: a core
// Service converted single- to dual-stack moves nothing else the framework
// compares.
func TestOverlayInputsHash_CoversClusterOverlayInputs(t *testing.T) {
	const gatewayName = "ingress"
	namespaces := krt.NewStaticCollection(nil, []krtcollections.NamespaceMetadata{
		{Name: "ns"},
		{Name: "labeled-ns", Labels: map[string]string{wellknown.IngressUseWaypointLabel: "true"}},
	})
	gateways := krt.NewStaticCollection(nil, []ir.Gateway{{
		ObjectSource: ir.ObjectSource{
			Group: wellknown.GatewayGVK.Group, Kind: wellknown.GatewayGVK.Kind,
			Namespace: "ns", Name: gatewayName,
		},
		Obj: &gwv1.Gateway{Spec: gwv1.GatewaySpec{GatewayClassName: "kgateway"}},
	}})
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns", Name: "svc", UID: "uid", ResourceVersion: "1",
			Labels: map[string]string{wellknown.IngressUseWaypointLabel: "true"},
		},
		Spec: corev1.ServiceSpec{ClusterIP: "10.0.0.1", ClusterIPs: []string{"10.0.0.1"}},
	}
	waypoint := types.NamespacedName{Namespace: "ns", Name: "waypoint"}
	p := &PerClientProcessor{
		waypointQueries: fakeWaypointQueries{attached: map[string]types.NamespacedName{
			waypointquery.ServiceKeyFromObject(service): waypoint,
			// The renamed and moved services are attached too, so those
			// mutations exercise the lookup key rather than a missing waypoint.
			waypointquery.ServiceKeyFromObject(&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "renamed"}}):     waypoint,
			waypointquery.ServiceKeyFromObject(&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "labeled-ns", Name: "svc"}}): waypoint,
		}},
		commonCols: &collections.CommonCollections{
			Namespaces:   namespaces,
			GatewayIndex: &krtcollections.GatewayIndex{Gateways: gateways},
		},
		waypointGatewayClassName: "istio-waypoint",
	}

	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: "", Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")
	backend.Obj = service
	backend.CanonicalHostname = "svc.ns.svc.cluster.local"

	ambient := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{
		istioannot.AmbientRedirection.Name: "enabled",
		wellknown.GatewayNameLabel:         gatewayName,
	}, ir.PodLocality{})
	plain := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})

	overlaytest.AssertInputsHashCoversOverlay(t, overlaytest.Case{
		Plugin:  p.policyPlugin(),
		Backend: backend,
		Base:    &envoyclusterv3.Cluster{Name: backend.ClusterName(), ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}},
		Clients: []ir.UniquelyConnectedClient{ambient, plain},
		Mutations: []overlaytest.Mutation{
			overlaytest.RemoveLabel(t, wellknown.IngressUseWaypointLabel),
			{Name: "unlabeled object in a labeled namespace", Apply: func(b *ir.BackendObjectIR) {
				svc := overlaytest.CloneObject(t, b.Obj).(*corev1.Service)
				svc.Labels = nil
				svc.Namespace = "labeled-ns"
				b.Obj = svc
			}},
			{Name: "renamed object", Apply: func(b *ir.BackendObjectIR) {
				svc := overlaytest.CloneObject(t, b.Obj).(*corev1.Service)
				svc.Name = "renamed"
				b.Obj = svc
			}},
			{Name: "object nothing attaches a waypoint to", Apply: func(b *ir.BackendObjectIR) {
				svc := overlaytest.CloneObject(t, b.Obj).(*corev1.Service)
				svc.Name = "detached"
				b.Obj = svc
			}},
			{Name: "dual-stack clusterIPs", Apply: func(b *ir.BackendObjectIR) {
				svc := overlaytest.CloneObject(t, b.Obj).(*corev1.Service)
				svc.Spec.ClusterIPs = []string{"10.0.0.1", "2001:db8::1"}
				b.Obj = svc
			}},
			{Name: "headless", Apply: func(b *ir.BackendObjectIR) {
				svc := overlaytest.CloneObject(t, b.Obj).(*corev1.Service)
				svc.Spec.ClusterIP = corev1.ClusterIPNone
				svc.Spec.ClusterIPs = []string{corev1.ClusterIPNone}
				b.Obj = svc
			}},
			{Name: "port", Apply: func(b *ir.BackendObjectIR) {
				rebuilt := ir.NewBackendObjectIR(b.GetObjectSource(), 8080, "", "")
				rebuilt.Obj = b.Obj
				rebuilt.CanonicalHostname = b.CanonicalHostname
				*b = rebuilt
			}},
			{Name: "alias in a labeled namespace, own label absent", Apply: func(b *ir.BackendObjectIR) {
				svc := overlaytest.CloneObject(t, b.Obj).(*corev1.Service)
				svc.Labels = nil
				b.Obj = svc
				b.Aliases = []ir.ObjectSource{{Group: "networking.istio.io", Kind: "ServiceEntry", Namespace: "labeled-ns", Name: "se"}}
			}},
			overlaytest.SetLabel(t, "app", "svc"),
			overlaytest.SetAnnotation(t, "meta.helm.sh/release-name", "x"),
			overlaytest.SetResourceVersion(t, "2"),
		},
	})
}

// TestIngressUseWaypointClusterInputsHash_CoversApply checks the exported
// helper against the exported mutation it describes. Out-of-tree overlays call
// ApplyIngressUseWaypointCluster under their own gates and declare its inputs
// with this helper, so the helper alone must cover every backend field the
// mutation reads. The overlay here applies unconditionally so that only the
// mutation's own reads are under test.
func TestIngressUseWaypointClusterInputsHash_CoversApply(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "svc", UID: "uid", ResourceVersion: "1"},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.1", ClusterIPs: []string{"10.0.0.1"}},
	}
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")
	backend.Obj = service

	settings := &apisettings.Settings{DnsLookupFamily: apisettings.DnsLookupFamilyV4Preferred}
	overlaytest.AssertInputsHashCoversOverlay(t, overlaytest.Case{
		Plugin: sdk.PolicyPlugin{
			Name: "apply-only",
			PerClientClusterOverlay: func(_ krt.HandlerContext, _ context.Context, _ ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
				return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
					ApplyIngressUseWaypointCluster(in, out, settings)
				}}
			},
			OverlayInputsHash: func(in ir.BackendObjectIR) uint64 {
				hasher := fnv.New64a()
				IngressUseWaypointClusterInputsHash(hasher, in)
				return hasher.Sum64()
			},
		},
		Backend: backend,
		Clients: []ir.UniquelyConnectedClient{ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})},
		Mutations: []overlaytest.Mutation{
			{Name: "dual-stack clusterIPs", Apply: func(b *ir.BackendObjectIR) {
				svc := overlaytest.CloneObject(t, b.Obj).(*corev1.Service)
				svc.Spec.ClusterIPs = []string{"10.0.0.1", "2001:db8::1"}
				b.Obj = svc
			}},
			{Name: "port", Apply: func(b *ir.BackendObjectIR) {
				rebuilt := ir.NewBackendObjectIR(b.GetObjectSource(), 8080, "", "")
				rebuilt.Obj = b.Obj
				*b = rebuilt
			}},
			overlaytest.SetLabel(t, "app", "svc"),
			overlaytest.SetResourceVersion(t, "2"),
		},
	})
}

func TestWaypointRedirectClearsInheritedLocalityMode(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")
	backend.Obj = &corev1.Service{Spec: corev1.ServiceSpec{ClusterIP: "10.0.0.1", ClusterIPs: []string{"10.0.0.1"}}}
	out := &envoyclusterv3.Cluster{
		Name: backend.ClusterName(),
		CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
			HealthyPanicThreshold: &envoytypev3.Percent{Value: 25},
			LocalityConfigSpecifier: &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
				LocalityWeightedLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
			},
		},
	}
	ApplyIngressUseWaypointCluster(backend, out, &apisettings.Settings{})
	require.Nil(t, out.GetCommonLbConfig().GetLocalityConfigSpecifier(), "service VIP endpoints have no locality weights")
	require.Equal(t, float64(25), out.GetCommonLbConfig().GetHealthyPanicThreshold().GetValue(), "unrelated LB settings survive")
	require.Len(t, out.GetLoadAssignment().GetEndpoints(), 1)
}
