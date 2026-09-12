package krtcollections_test

import (
	"context"
	"fmt"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	. "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestUniqueClients(t *testing.T) {
	// Disable the first-connect delay: this test drives many new streams
	// through OnStreamRequest and doesn't exercise snapshot publication.
	t.Cleanup(SetXdsFirstConnectDelayForTest(0))

	testCases := []struct {
		name     string
		inputs   []any
		requests []*envoy_service_discovery_v3.DiscoveryRequest
		result   sets.Set[string]
	}{
		{
			name: "basic",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "podname",
						Namespace: "ns",
						Labels:    map[string]string{"a": "b"},
					},
					Spec: corev1.PodSpec{
						NodeName: "node",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node",
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone",
						},
					},
				},
			},
			requests: []*envoy_service_discovery_v3.DiscoveryRequest{
				{
					Node: &envoycorev3.Node{
						Id: "podname.ns",
						Metadata: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								xds.RoleKey: structpb.NewStringValue(wellknown.GatewayApiProxyValue + "~best-proxy-role"),
							},
						},
					},
				},
			},
			result: sets.New(
				fmt.Sprintf("kgateway-kube-gateway-api~best-proxy-role~%d~ns", utils.HashLabels(map[string]string{
					corev1.LabelTopologyRegion: "region",
					corev1.LabelTopologyZone:   "zone",
					corev1.LabelHostname:       "node",
					"a":                        "b",
				})),
			),
		},
		{
			name: "two UCCs",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "podname",
						Namespace: "ns",
						Labels:    map[string]string{"a": "b"},
					},
					Spec: corev1.PodSpec{
						NodeName: "node",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node",
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone",
						},
					},
				},
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "podname2",
						Namespace: "ns",
						Labels:    map[string]string{"a": "b"},
					},
					Spec: corev1.PodSpec{
						NodeName: "node2",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region2",
							corev1.LabelTopologyZone:   "zone2",
						},
					},
				},
			},
			requests: []*envoy_service_discovery_v3.DiscoveryRequest{
				{
					Node: &envoycorev3.Node{
						Id: "podname.ns",
						Metadata: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								xds.RoleKey: structpb.NewStringValue(wellknown.GatewayApiProxyValue + "~best-proxy-role"),
							},
						},
					},
				},
				{
					Node: &envoycorev3.Node{
						Id: "podname2.ns",
						Metadata: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								xds.RoleKey: structpb.NewStringValue(wellknown.GatewayApiProxyValue + "~best-proxy-role"),
							},
						},
					},
				},
			},
			result: sets.New(
				fmt.Sprintf("kgateway-kube-gateway-api~best-proxy-role~%d~ns", utils.HashLabels(map[string]string{
					corev1.LabelTopologyRegion: "region",
					corev1.LabelTopologyZone:   "zone",
					corev1.LabelHostname:       "node",
					"a":                        "b",
				})), fmt.Sprintf("kgateway-kube-gateway-api~best-proxy-role~%d~ns", utils.HashLabels(map[string]string{
					corev1.LabelTopologyRegion: "region2",
					corev1.LabelTopologyZone:   "zone2",
					corev1.LabelHostname:       "node2",
					"a":                        "b",
				})),
			),
		},
		{
			name:   "no-pods",
			inputs: nil,
			requests: []*envoy_service_discovery_v3.DiscoveryRequest{
				{
					Node: &envoycorev3.Node{
						Id: "podname.ns",
						Metadata: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								xds.RoleKey: structpb.NewStringValue(wellknown.GatewayApiProxyValue + "~best-proxy-role"),
							},
						},
					},
				},
			},
			result: sets.New(wellknown.GatewayApiProxyValue + "~best-proxy-role"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fmt.Printf("start test %s\n", tc.name)
			g := NewWithT(t)
			var pods krt.Collection[LocalityPod]
			if tc.inputs != nil {
				mock := krttest.NewMock(t, tc.inputs)
				nodes := NewNodeMetadataCollection(krttest.GetMockCollection[*corev1.Node](mock))
				pods = NewLocalityPodsCollection(nodes, krttest.GetMockCollection[*corev1.Pod](mock), krtutil.KrtOptions{})
				nodes.WaitUntilSynced(context.Background().Done())
				pods.WaitUntilSynced(context.Background().Done())
			}

			cb, uccBuilder := NewUniquelyConnectedClients(nil, false)
			ucc := uccBuilder(context.Background(), krtutil.KrtOptions{}, pods)
			ucc.WaitUntilSynced(context.Background().Done())

			// check fetch as well
			fetchNames := sets.New[string]()

			for i, r := range tc.requests {
				fetchDR := proto.Clone(r).(*envoy_service_discovery_v3.DiscoveryRequest)
				err := cb.OnFetchRequest(context.Background(), fetchDR)
				g.Expect(err).NotTo(HaveOccurred())
				fetchNames.Insert(fetchDR.GetNode().GetMetadata().GetFields()[xds.RoleKey].GetStringValue())

				for j := range 10 { // simulate 10 requests that are the same client
					cb.OnStreamRequest(int64(i*10+j), proto.Clone(r).(*envoy_service_discovery_v3.DiscoveryRequest))
				}
			}

			// propagating the event happens async
			var allUcc []ir.UniquelyConnectedClient
			g.Eventually(func() []ir.UniquelyConnectedClient {
				allUcc = ucc.List()
				return allUcc
			}, "1s").Should(HaveLen(len(tc.result)))

			names := sets.New[string]()
			for _, uc := range allUcc {
				names.Insert(uc.ResourceName())
			}
			g.Expect(fetchNames).To(Equal(tc.result))
			g.Expect(names).To(Equal(tc.result))

			for i := range tc.requests {
				for j := range 9 {
					cb.OnStreamClosed(int64(i*10+j), nil)
				}
			}

			g.Expect(ucc.List()).Should(HaveLen(len(tc.result)))

			for i := range tc.requests {
				j := 9
				g.Eventually(ucc.List).Should(HaveLen(len(allUcc) - i))
				cb.OnStreamClosed(int64(i*10+j), nil)
			}

			// as events happens async, eventually after all clients disconnect all UCCs should be removed
			g.Eventually(func() []ir.UniquelyConnectedClient {
				allUcc = ucc.List()
				return allUcc
			}, "5s").Should(BeEmpty())
		})
	}
}

// TestUniqueClientsLocalClusterCapabilityGating guards against #14471: kgateway must not
// assume a connected client (Envoy) knows about the per-gateway "local cluster" EDS resource
// until that client's own EDS subscription actually names it. Old Envoys never do (no matching
// static bootstrap cluster), and handing them the resource anyway makes go-control-plane's ADS
// "superset" check withhold their entire EDS response, not just the local cluster.
func TestUniqueClientsLocalClusterCapabilityGating(t *testing.T) {
	t.Cleanup(SetXdsFirstConnectDelayForTest(0))
	g := NewWithT(t)

	inputs := []any{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "podname",
				Namespace: "ns",
				Labels: map[string]string{
					wellknown.GatewayNameLabel: "gw",
				},
			},
			Spec: corev1.PodSpec{NodeName: "node"},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node",
				Labels: map[string]string{
					corev1.LabelTopologyRegion: "region",
					corev1.LabelTopologyZone:   "zone",
				},
			},
		},
	}
	mock := krttest.NewMock(t, inputs)
	nodes := NewNodeMetadataCollection(krttest.GetMockCollection[*corev1.Node](mock))
	pods := NewLocalityPodsCollection(nodes, krttest.GetMockCollection[*corev1.Pod](mock), krtutil.KrtOptions{})
	nodes.WaitUntilSynced(context.Background().Done())
	pods.WaitUntilSynced(context.Background().Done())

	cb, uccBuilder := NewUniquelyConnectedClients(nil, false)
	uccCol := uccBuilder(context.Background(), krtutil.KrtOptions{}, pods)
	uccCol.WaitUntilSynced(context.Background().Done())

	node := &envoycorev3.Node{
		Id: "podname.ns",
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				xds.RoleKey: structpb.NewStringValue(wellknown.GatewayApiProxyValue + "~best-proxy-role"),
			},
		},
	}

	// Old-style client: its EDS subscription names its normal backend cluster, but never the
	// local-cluster resource (its bootstrap has no matching static cluster to ask for).
	err := cb.OnStreamRequest(1, &envoy_service_discovery_v3.DiscoveryRequest{
		Node:          node,
		TypeUrl:       resource.EndpointType,
		ResourceNames: []string{"some-backend-cluster"},
	})
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(uccCol.List).Should(HaveLen(1))
	g.Consistently(func() bool {
		return uccCol.List()[0].KnowsLocalCluster
	}).Should(BeFalse(), "must not assume support before the client actually asks for the resource")

	// New-style client on the same stream now also names the local cluster resource.
	err = cb.OnStreamRequest(1, &envoy_service_discovery_v3.DiscoveryRequest{
		Node:          node,
		TypeUrl:       resource.EndpointType,
		ResourceNames: []string{"some-backend-cluster", "gw.ns"},
	})
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(func() bool {
		list := uccCol.List()
		return len(list) == 1 && list[0].KnowsLocalCluster
	}).Should(BeTrue())
}

// TestUniqueClientsLocalClusterCapabilityGatingSharedBucket guards against a narrower case of
// #14471: when pod-locality tracking is disabled (DISABLE_POD_LOCALITY_XDS=true), every stream
// for a given role shares a single UCC bucket/snapshot, so KnowsLocalCluster must reflect
// whether ALL streams currently in that bucket have confirmed support -- not just one of them.
// A single un-confirmed sibling must hold the whole bucket back, even if another sibling
// already proved support; and a brand-new sibling connecting (even one that will go on to
// confirm) transiently un-confirms an already-confirmed bucket until it too proves support,
// since the bucket's single shared snapshot can't offer the resource to a subset of its
// streams. This is expected: the alternative (assuming a newcomer supports it before it says
// so) is exactly the bug #14471 was filed for.
func TestUniqueClientsLocalClusterCapabilityGatingSharedBucket(t *testing.T) {
	t.Cleanup(SetXdsFirstConnectDelayForTest(0))
	g := NewWithT(t)

	// role is deliberately 3 parts (prefix~ns~gateway) so ir.UniquelyConnectedClient.
	// LocalClusterInfo can fall back to deriving namespace/gateway from the role when there's
	// no pod (and therefore no namespace/labels) to derive them from directly.
	role := wellknown.GatewayApiProxyValue + "~ns~gw"
	nodeFor := func(id string) *envoycorev3.Node {
		return &envoycorev3.Node{
			Id: id,
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					xds.RoleKey: structpb.NewStringValue(role),
				},
			},
		}
	}

	cb, uccBuilder := NewUniquelyConnectedClients(nil, false)
	var pods krt.Collection[LocalityPod] // nil: pod-locality tracking disabled, so all streams for this role share one bucket
	uccCol := uccBuilder(context.Background(), krtutil.KrtOptions{}, pods)
	uccCol.WaitUntilSynced(context.Background().Done())

	// sid 1 connects and immediately confirms support for the local cluster resource.
	err := cb.OnStreamRequest(1, &envoy_service_discovery_v3.DiscoveryRequest{
		Node:          nodeFor("sid1.ns"),
		TypeUrl:       resource.EndpointType,
		ResourceNames: []string{"some-backend-cluster", "gw.ns"},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Eventually(func() []ir.UniquelyConnectedClient {
		return uccCol.List()
	}).Should(HaveLen(1))
	g.Eventually(func() bool {
		return uccCol.List()[0].KnowsLocalCluster
	}).Should(BeTrue(), "the only stream in the bucket has confirmed support")

	// sid 2 joins the same bucket (same role) but hasn't sent an EDS request yet. The still-
	// shared bucket must go back to un-confirmed, even though sid 1 already proved support --
	// the single snapshot they share can't offer the resource to sid 1 alone.
	err = cb.OnStreamRequest(2, &envoy_service_discovery_v3.DiscoveryRequest{
		Node:    nodeFor("sid2.ns"),
		TypeUrl: resource.ClusterType,
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Eventually(func() []ir.UniquelyConnectedClient {
		return uccCol.List()
	}).Should(HaveLen(1), "sid 1 and sid 2 must share a single bucket")
	g.Eventually(func() bool {
		return uccCol.List()[0].KnowsLocalCluster
	}).Should(BeFalse(), "sid 2 hasn't confirmed support yet, so the shared bucket must be held back")
	g.Consistently(func() bool {
		return uccCol.List()[0].KnowsLocalCluster
	}).Should(BeFalse(), "sid 2 still hasn't confirmed; the bucket must not flip back on its own")

	// sid 2 now also confirms support via its own EDS request; the bucket should flip back to
	// confirmed since every stream sharing it now supports the resource.
	err = cb.OnStreamRequest(2, &envoy_service_discovery_v3.DiscoveryRequest{
		Node:          nodeFor("sid2.ns"),
		TypeUrl:       resource.EndpointType,
		ResourceNames: []string{"gw.ns"},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Eventually(func() bool {
		list := uccCol.List()
		return len(list) == 1 && list[0].KnowsLocalCluster
	}).Should(BeTrue(), "every stream sharing the bucket has now confirmed support")

	// sid 2 disconnects; the bucket must stay confirmed since the one remaining stream (sid 1)
	// already confirmed support.
	cb.OnStreamClosed(2, nil)
	g.Consistently(func() bool {
		list := uccCol.List()
		return len(list) == 1 && list[0].KnowsLocalCluster
	}).Should(BeTrue(), "the remaining stream already confirmed support")
}

// A stream's identity is derived from pod data that can be stale at connect
// time (informer lag during controller start — exactly when every Envoy
// reconnects). The identity cannot be changed in place for an open stream
// (the snapshot cache key is bound to it), so when the freshly derived
// identity differs, the stream must be REJECTED so the client reconnects and
// re-identifies against current state — instead of serving wrong
// locality/label-derived config until an Envoy restart.
// go-control-plane reuses the first request's Node object for follow-up SotW
// requests that omit Node — including the role newStream rewrote in place to
// the unique cache key. Follow-up identity re-derivation must start from the
// stream's pinned original role: otherwise, for pods without a gateway-name
// label (where NormalizeGatewayRole is a passthrough), the already-augmented
// role re-augments into a different resource name and every ACK closes the
// stream as a false identity change.
func TestUniqueClientsFollowUpWithReusedAugmentedNode(t *testing.T) {
	t.Cleanup(SetXdsFirstConnectDelayForTest(0))
	g := NewWithT(t)
	ctx := context.Background()

	role := wellknown.GatewayApiProxyValue + "~best-proxy-role"
	labels := map[string]string{"a": "b"} // deliberately no gateway-name label
	driftedLabels := map[string]string{"a": "b", corev1.LabelTopologyZone: "zone-1"}

	pods := krt.NewStaticCollection[LocalityPod](nil, []LocalityPod{{
		Named:           krt.Named{Name: "podname", Namespace: "ns"},
		AugmentedLabels: labels,
	}})

	cb, uccBuilder := NewUniquelyConnectedClients(nil, false)
	ucc := uccBuilder(ctx, krtutil.KrtOptions{}, pods)
	ucc.WaitUntilSynced(ctx.Done())

	req := &envoy_service_discovery_v3.DiscoveryRequest{
		Node: &envoycorev3.Node{
			Id: "podname.ns",
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					xds.RoleKey: structpb.NewStringValue(role),
				},
			},
		},
	}
	uniqueName := fmt.Sprintf("%s~%d~ns", role, utils.HashLabels(labels))

	// The first request rewrites req's Node role in place to the unique key.
	g.Expect(cb.OnStreamRequest(1, req)).To(Succeed())
	g.Expect(req.GetNode().GetMetadata().GetFields()[xds.RoleKey].GetStringValue()).To(Equal(uniqueName),
		"newStream must have augmented the node role in place")

	// Follow-ups reuse the SAME mutated request object (as go-control-plane
	// does). They must not read as identity changes.
	for range 3 {
		g.Expect(cb.OnStreamRequest(1, req)).To(Succeed(),
			"an ACK carrying the reused augmented node must not close the stream")
	}
	g.Eventually(func() []ir.UniquelyConnectedClient { return ucc.List() }, "1s").Should(HaveLen(1))
	g.Expect(ucc.List()[0].ResourceName()).To(Equal(uniqueName))

	// Genuine pod-state drift must still be detected through the reused node.
	pods.UpdateObject(LocalityPod{
		Named:           krt.Named{Name: "podname", Namespace: "ns"},
		AugmentedLabels: driftedLabels,
	})
	g.Expect(cb.OnStreamRequest(1, req)).To(MatchError(ContainSubstring("xds client identity changed")),
		"real label drift must still close the stream even with a reused node")
}

func TestUniqueClientsReidentifyOnPodChange(t *testing.T) {
	t.Cleanup(SetXdsFirstConnectDelayForTest(0))
	g := NewWithT(t)
	ctx := context.Background()

	role := wellknown.GatewayApiProxyValue + "~best-proxy-role"
	staleLabels := map[string]string{"a": "b"}
	freshLabels := map[string]string{"a": "b", corev1.LabelTopologyZone: "zone-1"}

	pods := krt.NewStaticCollection[LocalityPod](nil, []LocalityPod{{
		Named:           krt.Named{Name: "podname", Namespace: "ns"},
		AugmentedLabels: staleLabels,
	}})

	cb, uccBuilder := NewUniquelyConnectedClients(nil, false)
	ucc := uccBuilder(ctx, krtutil.KrtOptions{}, pods)
	ucc.WaitUntilSynced(ctx.Done())

	req := &envoy_service_discovery_v3.DiscoveryRequest{
		Node: &envoycorev3.Node{
			Id: "podname.ns",
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					xds.RoleKey: structpb.NewStringValue(role),
				},
			},
		},
	}
	cloneReq := func() *envoy_service_discovery_v3.DiscoveryRequest {
		return proto.Clone(req).(*envoy_service_discovery_v3.DiscoveryRequest)
	}

	staleName := fmt.Sprintf("%s~%d~ns", role, utils.HashLabels(staleLabels))
	freshName := fmt.Sprintf("%s~%d~ns", role, utils.HashLabels(freshLabels))

	// First contact freezes the identity derived from current (stale) data.
	g.Expect(cb.OnStreamRequest(1, cloneReq())).To(Succeed())
	g.Eventually(func() []ir.UniquelyConnectedClient { return ucc.List() }, "1s").Should(HaveLen(1))
	g.Expect(ucc.List()[0].ResourceName()).To(Equal(staleName))

	// The pod's augmented data catches up while the stream is open.
	pods.UpdateObject(LocalityPod{
		Named:           krt.Named{Name: "podname", Namespace: "ns"},
		AugmentedLabels: freshLabels,
	})

	// The next request on the SAME stream re-derives identity, detects the
	// drift, and rejects the stream so the client re-identifies.
	err := cb.OnStreamRequest(1, cloneReq())
	g.Expect(err).To(MatchError(fmt.Sprintf("xds client identity changed from %q to %q", staleName, freshName)),
		"a drifted identity must close the stream")

	// The reconnect (new stream id) identifies against fresh data.
	cb.OnStreamClosed(1, nil)
	g.Expect(cb.OnStreamRequest(2, cloneReq())).To(Succeed())
	g.Eventually(func() sets.Set[string] {
		names := sets.New[string]()
		for _, c := range ucc.List() {
			names.Insert(c.ResourceName())
		}
		return names
	}, "1s").Should(Equal(sets.New(freshName)), "the reconnected stream must carry the fresh identity and the stale one must be gone")
}

// A derivation failure on an ESTABLISHED stream (pod record absent from the
// collection: informer blip, force-deleted pod, node lost) must not close the
// stream — the client keeps serving under its established identity until
// derivation succeeds again. This pins the derr-tolerated branch in add(); a
// reorder that surfaces derr before the established-stream check would turn
// every informer blip into cluster-wide stream churn.
func TestUniqueClientsKeepIdentityWhenPodLookupFails(t *testing.T) {
	t.Cleanup(SetXdsFirstConnectDelayForTest(0))
	g := NewWithT(t)
	ctx := context.Background()

	role := wellknown.GatewayApiProxyValue + "~best-proxy-role"
	labels := map[string]string{"a": "b"}

	pods := krt.NewStaticCollection[LocalityPod](nil, []LocalityPod{{
		Named:           krt.Named{Name: "podname", Namespace: "ns"},
		AugmentedLabels: labels,
	}})

	cb, uccBuilder := NewUniquelyConnectedClients(nil, false)
	ucc := uccBuilder(ctx, krtutil.KrtOptions{}, pods)
	ucc.WaitUntilSynced(ctx.Done())

	req := &envoy_service_discovery_v3.DiscoveryRequest{
		Node: &envoycorev3.Node{
			Id: "podname.ns",
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					xds.RoleKey: structpb.NewStringValue(role),
				},
			},
		},
	}
	cloneReq := func() *envoy_service_discovery_v3.DiscoveryRequest {
		return proto.Clone(req).(*envoy_service_discovery_v3.DiscoveryRequest)
	}

	uniqueName := fmt.Sprintf("%s~%d~ns", role, utils.HashLabels(labels))

	// Establish the stream while the pod is resolvable.
	g.Expect(cb.OnStreamRequest(1, cloneReq())).To(Succeed(), "first contact with the pod present must succeed")
	g.Eventually(func() []ir.UniquelyConnectedClient { return ucc.List() }, "1s").Should(HaveLen(1),
		"the client must be registered")

	// The pod record disappears while the stream stays open.
	pods.DeleteObject(krt.Named{Name: "podname", Namespace: "ns"}.ResourceName())

	// Follow-ups keep serving under the established identity instead of
	// churning the stream.
	for range 3 {
		g.Expect(cb.OnStreamRequest(1, cloneReq())).To(Succeed(),
			"a pod-lookup failure on an established stream must not close it")
	}
	g.Expect(ucc.List()).To(HaveLen(1), "the established client must remain registered")
	g.Expect(ucc.List()[0].ResourceName()).To(Equal(uniqueName), "the established identity must be retained")

	// A NEW stream during the same outage is still rejected (no identity to
	// fall back on).
	g.Expect(cb.OnStreamRequest(2, cloneReq())).To(MatchError(ContainSubstring("pod not found for node")),
		"a first request without a resolvable pod must be rejected")
}

// TestUniqueClientsLocalClusterGatingSurvivesIdentityReDerivation is the seam between
// per-request identity re-derivation (#14244) and the per-stream local-cluster capability
// gate (#14471/#14472): re-derivation must not silently drop a stream's already-proven
// capability, and a stream closed for identity drift must come back UNCONFIRMED so the
// old-Envoy blackhole can't be recreated for the new identity.
func TestUniqueClientsLocalClusterGatingSurvivesIdentityReDerivation(t *testing.T) {
	t.Cleanup(SetXdsFirstConnectDelayForTest(0))
	g := NewWithT(t)
	ctx := context.Background()

	role := wellknown.GatewayApiProxyValue + "~best-proxy-role"
	labels := map[string]string{wellknown.GatewayNameLabel: "gw"}
	driftedLabels := map[string]string{wellknown.GatewayNameLabel: "gw", corev1.LabelTopologyZone: "zone-1"}

	pods := krt.NewStaticCollection[LocalityPod](nil, []LocalityPod{{
		Named:           krt.Named{Name: "podname", Namespace: "ns"},
		AugmentedLabels: labels,
	}})

	cb, uccBuilder := NewUniquelyConnectedClients(nil, false)
	uccCol := uccBuilder(ctx, krtutil.KrtOptions{}, pods)
	uccCol.WaitUntilSynced(ctx.Done())

	edsReq := func() *envoy_service_discovery_v3.DiscoveryRequest {
		return &envoy_service_discovery_v3.DiscoveryRequest{
			Node: &envoycorev3.Node{
				Id: "podname.ns",
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						xds.RoleKey: structpb.NewStringValue(role),
					},
				},
			},
			TypeUrl:       resource.EndpointType,
			ResourceNames: []string{"some-backend-cluster", "gw.ns"},
		}
	}

	// The client proves local-cluster support on its first EDS request.
	g.Expect(cb.OnStreamRequest(1, edsReq())).To(Succeed())
	g.Eventually(func() bool {
		list := uccCol.List()
		return len(list) == 1 && list[0].KnowsLocalCluster
	}).Should(BeTrue(), "the stream named the local cluster resource")

	// Re-derivation on subsequent requests must leave the confirmed capability alone.
	g.Expect(cb.OnStreamRequest(1, edsReq())).To(Succeed())
	g.Consistently(func() bool {
		list := uccCol.List()
		return len(list) == 1 && list[0].KnowsLocalCluster
	}).Should(BeTrue(), "re-derivation must not drop a stream's proven capability")

	// Identity drift closes the stream; the capability must NOT carry over to the
	// reconnect, which is a fresh stream that has not yet proven anything.
	pods.UpdateObject(LocalityPod{
		Named:           krt.Named{Name: "podname", Namespace: "ns"},
		AugmentedLabels: driftedLabels,
	})
	g.Expect(cb.OnStreamRequest(1, edsReq())).To(MatchError(ContainSubstring("xds client identity changed")))
	cb.OnStreamClosed(1, nil)

	// The reconnected stream sends CDS first, so it hasn't named the local cluster yet.
	g.Expect(cb.OnStreamRequest(2, &envoy_service_discovery_v3.DiscoveryRequest{
		Node: &envoycorev3.Node{
			Id: "podname.ns",
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					xds.RoleKey: structpb.NewStringValue(role),
				},
			},
		},
		TypeUrl: resource.ClusterType,
	})).To(Succeed())

	// Assert on the DRIFTED identity by name: the pre-close snapshot still lists the
	// old (confirmed) identity for a moment, and a bare len()==1 check would match it.
	driftedName := fmt.Sprintf("kgateway-kube-gateway-api~ns~gw~%d~ns", utils.HashLabels(driftedLabels))
	knowsLocalCluster := func(name string) func() (bool, error) {
		return func() (bool, error) {
			for _, c := range uccCol.List() {
				if c.ResourceName() == name {
					return c.KnowsLocalCluster, nil
				}
			}
			return false, fmt.Errorf("no client named %q yet", name)
		}
	}
	g.Eventually(func() error {
		_, err := knowsLocalCluster(driftedName)()
		return err
	}, "1s").Should(Succeed(), "the reconnected stream must register under the drifted identity")
	g.Consistently(knowsLocalCluster(driftedName)).Should(BeFalse(),
		"a stream re-identified after drift must re-prove local-cluster support")

	// ...and it does, on its next EDS request.
	g.Expect(cb.OnStreamRequest(2, edsReq())).To(Succeed())
	g.Eventually(knowsLocalCluster(driftedName)).Should(BeTrue())
}

func TestNormalizeGatewayRole(t *testing.T) {
	testCases := []struct {
		name         string
		originalRole string
		namespace    string
		labels       map[string]string
		expectedRole string
	}{
		{
			name:         "nil labels returns original role unchanged",
			originalRole: "original-role",
			namespace:    "test-ns",
			labels:       nil,
			expectedRole: "original-role",
		},
		{
			name:         "labels with GatewayNameAnnotation returns constructed role",
			originalRole: "original-role",
			namespace:    "test-ns",
			labels: map[string]string{
				wellknown.GatewayNameAnnotation: "my-gateway",
			},
			expectedRole: "kgateway-kube-gateway-api~test-ns~my-gateway",
		},
		{
			name:         "labels with GatewayNameLabel returns constructed role",
			originalRole: "original-role",
			namespace:    "test-ns",
			labels: map[string]string{
				wellknown.GatewayNameLabel: "my-gateway",
			},
			expectedRole: "kgateway-kube-gateway-api~test-ns~my-gateway",
		},
		{
			name:         "labels with both annotation and label uses annotation",
			originalRole: "original-role",
			namespace:    "test-ns",
			labels: map[string]string{
				wellknown.GatewayNameAnnotation: "gateway-from-annotation",
				wellknown.GatewayNameLabel:      "gateway-from-label",
			},
			expectedRole: "kgateway-kube-gateway-api~test-ns~gateway-from-annotation",
		},
		{
			name:         "labels without gateway name keys returns original role unchanged",
			originalRole: "original-role",
			namespace:    "test-ns",
			labels: map[string]string{
				"app": "my-app",
			},
			expectedRole: "original-role",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			result := NormalizeGatewayRole(tc.originalRole, tc.namespace, tc.labels)
			g.Expect(result).To(Equal(tc.expectedRole))
		})
	}
}
