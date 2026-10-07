package krtcollections_test

import (
	"context"
	"strings"
	"testing"

	"istio.io/api/label"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestPods(t *testing.T) {
	testCases := []struct {
		name   string
		inputs []any
		result krtcollections.LocalityPod
	}{
		{
			name: "basic",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "ns",
						Labels:    map[string]string{"a": "b"},
					},
					Spec: corev1.PodSpec{
						NodeName: "node",
					},
					Status: corev1.PodStatus{
						PodIP: "1.2.3.4",
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
			result: krtcollections.LocalityPod{
				Named: krt.Named{
					Name:      "name",
					Namespace: "ns",
				},
				Locality: ir.PodLocality{
					Region:  "region",
					Zone:    "zone",
					Subzone: "",
				},
				AugmentedLabels: map[string]string{
					corev1.LabelTopologyRegion: "region",
					corev1.LabelTopologyZone:   "zone",
					corev1.LabelHostname:       "node",
					"a":                        "b",
				},
				Addresses: []string{"1.2.3.4"},
			},
		},
		{
			name: "multi-IP",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "ns",
						Labels:    map[string]string{"a": "b"},
					},
					Spec: corev1.PodSpec{
						NodeName: "node",
					},
					Status: corev1.PodStatus{
						PodIP: "1.2.3.4",
						PodIPs: []corev1.PodIP{
							{IP: "1.2.3.4"},
							{IP: "2001:db8::1"},
						},
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
			result: krtcollections.LocalityPod{
				Named: krt.Named{
					Name:      "name",
					Namespace: "ns",
				},
				Locality: ir.PodLocality{
					Region:  "region",
					Zone:    "zone",
					Subzone: "",
				},
				AugmentedLabels: map[string]string{
					corev1.LabelTopologyRegion: "region",
					corev1.LabelTopologyZone:   "zone",
					corev1.LabelHostname:       "node",
					"a":                        "b",
				},
				Addresses: []string{"1.2.3.4", "2001:db8::1"},
			},
		},
		{
			name: "no IP",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
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
			result: krtcollections.LocalityPod{
				Named: krt.Named{
					Name:      "name",
					Namespace: "ns",
				},
				Locality: ir.PodLocality{
					Region:  "region",
					Zone:    "zone",
					Subzone: "",
				},
				AugmentedLabels: map[string]string{
					corev1.LabelTopologyRegion: "region",
					corev1.LabelTopologyZone:   "zone",
					corev1.LabelHostname:       "node",
					"a":                        "b",
				},
			},
		},
		{
			name: "long gateway name annotation is augmented into labels",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "ns",
						Labels:    map[string]string{"a": "b"},
						Annotations: map[string]string{
							// This is a long gateway name that exceeds 63 chars
							wellknown.GatewayNameAnnotation: strings.Repeat("a", 100) + "-gateway",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node",
					},
					Status: corev1.PodStatus{
						PodIP: "1.2.3.4",
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
			result: krtcollections.LocalityPod{
				Named: krt.Named{
					Name:      "name",
					Namespace: "ns",
				},
				Locality: ir.PodLocality{
					Region:  "region",
					Zone:    "zone",
					Subzone: "",
				},
				AugmentedLabels: map[string]string{
					corev1.LabelTopologyRegion:      "region",
					corev1.LabelTopologyZone:        "zone",
					corev1.LabelHostname:            "node",
					"a":                             "b",
					wellknown.GatewayNameAnnotation: strings.Repeat("a", 100) + "-gateway",
				},
				Addresses: []string{"1.2.3.4"},
			},
		},
		{
			name: "pod without network label gets the system namespace network",
			inputs: []any{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "ns",
						Labels:    map[string]string{"a": "b"},
					},
					Status: corev1.PodStatus{
						PodIP: "1.2.3.4",
					},
				},
				istioSystemNamespace("cluster1"),
			},
			result: krtcollections.LocalityPod{
				Named: krt.Named{
					Name:      "name",
					Namespace: "ns",
				},
				AugmentedLabels: map[string]string{
					"a":                        "b",
					label.TopologyNetwork.Name: "cluster1",
				},
				Addresses: []string{"1.2.3.4"},
			},
		},
		{
			name: "pod network label wins over the system namespace network",
			inputs: []any{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "ns",
						Labels:    map[string]string{label.TopologyNetwork.Name: "other"},
					},
					Status: corev1.PodStatus{
						PodIP: "1.2.3.4",
					},
				},
				istioSystemNamespace("cluster1"),
			},
			result: krtcollections.LocalityPod{
				Named: krt.Named{
					Name:      "name",
					Namespace: "ns",
				},
				AugmentedLabels: map[string]string{
					label.TopologyNetwork.Name: "other",
				},
				Addresses: []string{"1.2.3.4"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			mock := krttest.NewMock(t, tc.inputs)
			nodes := krtcollections.NewNodeMetadataCollection(krttest.GetMockCollection[*corev1.Node](mock))
			namespaces := krtcollections.NewNamespaceCollectionFromCol(context.Background(), krttest.GetMockCollection[*corev1.Namespace](mock), krtutil.KrtOptions{})
			pods := krtcollections.NewLocalityPodsCollection(nodes, namespaces, "istio-system", krttest.GetMockCollection[*corev1.Pod](mock), krtutil.KrtOptions{})
			pods.WaitUntilSynced(context.Background().Done())
			lp := pods.List()[0]

			g.Expect(tc.result.Equals(lp)).To(BeTrue(), "expected %#v, got %#v", lp, tc.result)
		})
	}
}

func istioSystemNamespace(network string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "istio-system",
			Labels: map[string]string{label.TopologyNetwork.Name: network},
		},
	}
}
