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

	// Removing the only unconfirmed sibling must immediately restore the
	// capability proved by sid 1. This is why stream close always triggers a
	// recompute in the shared-bucket mode.
	cb.OnStreamClosed(2, nil)
	g.Eventually(func() bool {
		list := uccCol.List()
		return len(list) == 1 && list[0].KnowsLocalCluster
	}).Should(BeTrue(), "disconnecting the only unconfirmed stream must restore the shared capability")

	// Reconnect sid 2 so the all-stream confirmation transition below is still
	// exercised after the disconnect transition.
	err = cb.OnStreamRequest(2, &envoy_service_discovery_v3.DiscoveryRequest{
		Node:    nodeFor("sid2.ns"),
		TypeUrl: resource.ClusterType,
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Eventually(func() bool {
		return uccCol.List()[0].KnowsLocalCluster
	}).Should(BeFalse(), "the reconnected sibling must confirm support again")

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
