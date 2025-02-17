package krtcollections_test

import (
	"context"
	"fmt"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	. "github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
)

func TestUniqueClients(t *testing.T) {
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
					Node: &corev3.Node{
						Id: "podname.ns",
						Metadata: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								xds.RoleKey: structpb.NewStringValue(wellknown.GatewayApiProxyValue + "~best-proxy-role"),
							},
						},
					},
				},
			},
			result: sets.New(fmt.Sprintf("gloo-kube-gateway-api~best-proxy-role~%d~ns", utils.HashLabels(map[string]string{corev1.LabelTopologyRegion: "region", corev1.LabelTopologyZone: "zone", "a": "b"}))),
		},
		{
			name:   "no-pods",
			inputs: nil,
			requests: []*envoy_service_discovery_v3.DiscoveryRequest{
				{
					Node: &corev3.Node{
						Id: "podname.ns",
						Metadata: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								xds.RoleKey: structpb.NewStringValue(wellknown.GatewayApiProxyValue + "~best-proxy-role"),
							},
						},
					},
				},
			},
			result: sets.New(fmt.Sprintf(wellknown.GatewayApiProxyValue + "~best-proxy-role")),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var pods krt.Collection[LocalityPod]
			if tc.inputs != nil {
				mock := krttest.NewMock(t, tc.inputs)
				nodes := NewNodeMetadataCollection(krttest.GetMockCollection[*corev1.Node](mock))
				pods = NewLocalityPodsCollection(nodes, krttest.GetMockCollection[*corev1.Pod](mock), krtutil.KrtOptions{})
				pods.Synced().WaitUntilSynced(context.Background().Done())
			}

			cb, uccBuilder := NewUniquelyConnectedClients()
			ucc := uccBuilder(context.Background(), krtutil.KrtOptions{}, pods)
			ucc.Synced().WaitUntilSynced(context.Background().Done())

			// check fetch as well

			fetchNames := sets.New[string]()

			for i, r := range tc.requests {

				fetchDR := proto.Clone(r).(*envoy_service_discovery_v3.DiscoveryRequest)
				err := cb.OnFetchRequest(context.Background(), fetchDR)
				g.Expect(err).NotTo(HaveOccurred())
				fetchNames.Insert(fetchDR.GetNode().GetMetadata().GetFields()[xds.RoleKey].GetStringValue())

				for j := 0; j < 10; j++ { // simulate 10 requests that are the same client
					cb.OnStreamRequest(int64(i*10+j), proto.Clone(r).(*envoy_service_discovery_v3.DiscoveryRequest))
				}
			}

			allUcc := ucc.List()
			g.Expect(allUcc).To(HaveLen(len(tc.result)))
			names := sets.New[string]()
			for _, uc := range allUcc {
				names.Insert(uc.ResourceName())
			}
			g.Expect(fetchNames).To(Equal(tc.result))
			g.Expect(names).To(Equal(tc.result))
			for i := range tc.requests {
				for j := 0; j < 10; j++ {
					g.Expect(ucc.List()).To(HaveLen(len(allUcc) - i))
					cb.OnStreamClosed(int64(i*10+j), nil)
				}
				// make sure client removed only when all similar clients are removed.
				g.Expect(ucc.List()).To(HaveLen(len(allUcc) - 1 - i))
			}

		})
	}

}
