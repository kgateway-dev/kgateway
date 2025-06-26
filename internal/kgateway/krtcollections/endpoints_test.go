package krtcollections

import (
	"context"
	"testing"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func newBackendObjectIR(in ir.BackendObjectIR) ir.BackendObjectIR {
	src := in.ObjectSource
	port := in.Port
	extraKey := in.ExtraKey
	b := ir.NewBackendObjectIR(src, port, extraKey)
	b.Obj = in.Obj
	return b
}

func TestEndpointsForUpstreamOrderDoesntMatter(t *testing.T) {
	g := NewWithT(t)

	us := newBackendObjectIR(ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Namespace: "ns",
			Name:      "svc",
			Group:     "",
			Kind:      "Service",
		},
		Port: 8080,
		Obj: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc",
				Namespace: "ns",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "http",
						Port: 8080,
					},
				},
			},
		},
	})
	// input
	emd1 := ir.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &envoy_config_core_v3.Address{
						Address: &envoy_config_core_v3.Address_SocketAddress{
							SocketAddress: &envoy_config_core_v3.SocketAddress{
								Address: "1.2.3.4",
								PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
									PortValue: 8080,
								},
							},
						},
					},
				},
			},
		},
		EndpointMd: ir.EndpointMetadata{
			Labels: map[string]string{
				corev1.LabelTopologyRegion: "region",
				corev1.LabelTopologyZone:   "zone",
			},
		},
	}
	emd2 := ir.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &envoy_config_core_v3.Address{
						Address: &envoy_config_core_v3.Address_SocketAddress{
							SocketAddress: &envoy_config_core_v3.SocketAddress{
								Address: "1.2.3.5",
								PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
									PortValue: 8080,
								},
							},
						},
					},
				},
			},
		},
		EndpointMd: ir.EndpointMetadata{
			Labels: map[string]string{
				corev1.LabelTopologyRegion: "region2",
				corev1.LabelTopologyZone:   "zone2",
			},
		},
	}
	result1 := ir.NewEndpointsForBackend(us)
	result1.Add(ir.PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)
	result1.Add(ir.PodLocality{
		Region: "region2",
		Zone:   "zone2",
	}, emd2)

	result2 := ir.NewEndpointsForBackend(us)
	result2.Add(ir.PodLocality{
		Region: "region2",
		Zone:   "zone2",
	}, emd2)
	result2.Add(ir.PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)
	g.Expect(result1.Equals(*result2)).To(BeTrue(), "expected %v, got %v", result1, result2)

	// test with non matching locality
	result3 := ir.NewEndpointsForBackend(us)
	result3.Add(ir.PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)
	result3.Add(ir.PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd2)
	g.Expect(result1.Equals(*result3)).To(BeFalse(), "not expected %v, got %v", result1, result2)

	// test with non matching labels
	result4 := ir.NewEndpointsForBackend(us)
	result4.Add(ir.PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)

	emd2.EndpointMd.Labels["extra"] = "label"
	result4.Add(ir.PodLocality{
		Region: "region2",
		Zone:   "zone2",
	}, emd2)
	g.Expect(result1.Equals(*result4)).To(BeFalse(), "not expected %v, got %v", result1, result2)
}

func TestEndpointsForUpstreamWithDifferentNameButSameEndpoints(t *testing.T) {
	g := NewWithT(t)

	us := newBackendObjectIR(ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Namespace: "ns",
			Name:      "svc",
			Group:     "",
			Kind:      "Service",
		},
		Port: 8080,
		Obj: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc",
				Namespace: "ns",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "http",
						Port: 8080,
					},
				},
			},
		},
	})
	usd := newBackendObjectIR(ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Namespace: "ns",
			Name:      "discovered-name",
			Group:     "",
			Kind:      "Service",
		},
		Port: 8080,
		Obj: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "discovered-name",
				Namespace: "ns",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "http",
						Port: 8080,
					},
				},
			},
		},
	})
	// input
	emd1 := ir.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &envoy_config_core_v3.Address{
						Address: &envoy_config_core_v3.Address_SocketAddress{
							SocketAddress: &envoy_config_core_v3.SocketAddress{
								Address: "1.2.3.4",
								PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
									PortValue: 8080,
								},
							},
						},
					},
				},
			},
		},
		EndpointMd: ir.EndpointMetadata{
			Labels: map[string]string{
				corev1.LabelTopologyRegion: "region",
				corev1.LabelTopologyZone:   "zone",
			},
		},
	}
	emd2 := ir.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &envoy_config_core_v3.Address{
						Address: &envoy_config_core_v3.Address_SocketAddress{
							SocketAddress: &envoy_config_core_v3.SocketAddress{
								Address: "1.2.3.5",
								PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
									PortValue: 8080,
								},
							},
						},
					},
				},
			},
		},
		EndpointMd: ir.EndpointMetadata{
			Labels: map[string]string{
				corev1.LabelTopologyRegion: "region",
				corev1.LabelTopologyZone:   "zone",
			},
		},
	}

	result1 := ir.NewEndpointsForBackend(us)
	result1.Add(ir.PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)

	result2 := ir.NewEndpointsForBackend(usd)
	result2.Add(ir.PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)

	result3 := ir.NewEndpointsForBackend(us)
	result3.Add(ir.PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd2)

	result4 := ir.NewEndpointsForBackend(usd)
	result4.Add(ir.PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd2)

	h1 := result1.LbEpsEqualityHash ^ result2.LbEpsEqualityHash
	h2 := result3.LbEpsEqualityHash ^ result4.LbEpsEqualityHash

	g.Expect(h1).NotTo(Equal(h2), "not expected %v, got %v", h1, h2)
}

func TestEndpoints(t *testing.T) {
	testCases := []struct {
		name     string
		inputs   []any
		upstream ir.BackendObjectIR
		result   func(ir.BackendObjectIR) *ir.EndpointsForBackend
	}{
		{
			name: "basic",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "ns",
					},
					Spec: corev1.PodSpec{
						NodeName: "node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
				&discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-abcde", // Unique name for the EndpointSlice
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"1.2.3.4"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Pod",
								Name:      "name",
								Namespace: "ns",
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name:     ptr.To("http"),
							Port:     ptr.To(int32(8080)),
							Protocol: ptr.To(corev1.ProtocolTCP),
						},
					},
				},
			},

			upstream: newBackendObjectIR(ir.BackendObjectIR{
				ObjectSource: ir.ObjectSource{
					Namespace: "ns",
					Name:      "svc",
					Group:     "",
					Kind:      "Service",
				},
				Port: 8080,
				Obj: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "http",
								Port: 8080,
							},
						},
					},
				},
			}),
			result: func(us ir.BackendObjectIR) *ir.EndpointsForBackend {
				// output
				emd := ir.EndpointWithMd{
					LbEndpoint: &endpointv3.LbEndpoint{
						LoadBalancingWeight: wrapperspb.UInt32(1),
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
							Endpoint: &endpointv3.Endpoint{
								Address: &envoy_config_core_v3.Address{
									Address: &envoy_config_core_v3.Address_SocketAddress{
										SocketAddress: &envoy_config_core_v3.SocketAddress{
											Address: "1.2.3.4",
											PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
												PortValue: 8080,
											},
										},
									},
								},
							},
						},
					},
					EndpointMd: ir.EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone",
						},
					},
				}
				result := ir.NewEndpointsForBackend(us)
				result.Add(ir.PodLocality{
					Region: "region",
					Zone:   "zone",
				}, emd)
				return result
			},
		},
		{
			name: "two pods two zones",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "ns",
					},
					Spec: corev1.PodSpec{
						NodeName: "node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
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
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone2",
						},
					},
				},
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name2",
						Namespace: "ns",
					},
					Spec: corev1.PodSpec{
						NodeName: "node2",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						PodIP: "1.2.3.5",
					},
				},
				&discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-abcde",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"1.2.3.4"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Pod",
								Name:      "name",
								Namespace: "ns",
							},
						},
						{
							Addresses: []string{"1.2.3.5"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Pod",
								Name:      "name2",
								Namespace: "ns",
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name:     ptr.To("http"),
							Port:     ptr.To(int32(8080)),
							Protocol: ptr.To(corev1.ProtocolTCP),
						},
					},
				},
			},

			upstream: newBackendObjectIR(ir.BackendObjectIR{
				ObjectSource: ir.ObjectSource{
					Namespace: "ns",
					Name:      "svc",
					Group:     "",
					Kind:      "Service",
				},
				Port: 8080,
				Obj: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "http",
								Port: 8080,
							},
						},
					},
				},
			}),
			result: func(us ir.BackendObjectIR) *ir.EndpointsForBackend {
				// output
				result := ir.NewEndpointsForBackend(us)
				result.Add(ir.PodLocality{
					Region: "region",
					Zone:   "zone",
				}, ir.EndpointWithMd{
					LbEndpoint: &endpointv3.LbEndpoint{
						LoadBalancingWeight: wrapperspb.UInt32(1),
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
							Endpoint: &endpointv3.Endpoint{
								Address: &envoy_config_core_v3.Address{
									Address: &envoy_config_core_v3.Address_SocketAddress{
										SocketAddress: &envoy_config_core_v3.SocketAddress{
											Address: "1.2.3.4",
											PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
												PortValue: 8080,
											},
										},
									},
								},
							},
						},
					},
					EndpointMd: ir.EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone",
						},
					},
				})
				result.Add(ir.PodLocality{
					Region: "region",
					Zone:   "zone2",
				}, ir.EndpointWithMd{
					LbEndpoint: &endpointv3.LbEndpoint{
						LoadBalancingWeight: wrapperspb.UInt32(1),
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
							Endpoint: &endpointv3.Endpoint{
								Address: &envoy_config_core_v3.Address{
									Address: &envoy_config_core_v3.Address_SocketAddress{
										SocketAddress: &envoy_config_core_v3.SocketAddress{
											Address: "1.2.3.5",
											PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
												PortValue: 8080,
											},
										},
									},
								},
							},
						},
					},
					EndpointMd: ir.EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone2",
						},
					},
				})
				return result
			},
		},
		{
			name: "basic - metadata propagates",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "ns",
						Labels: map[string]string{
							// pod labels should propagate to endpoint metadata.
							"label": "value",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						PodIP: "1.2.3.4",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node",
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone",
							// this label should not propagate. only node topology labels should.
							"unralated": "label",
						},
					},
				},
				&discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-abcde",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"1.2.3.4"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Pod",
								Name:      "name",
								Namespace: "ns",
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name:     ptr.To("http"),
							Port:     ptr.To(int32(8080)),
							Protocol: ptr.To(corev1.ProtocolTCP),
						},
					},
				},
			},
			upstream: newBackendObjectIR(ir.BackendObjectIR{
				ObjectSource: ir.ObjectSource{
					Namespace: "ns",
					Name:      "svc",
					Group:     "",
					Kind:      "Service",
				},
				Port: 8080,
				Obj: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "http",
								Port: 8080,
							},
						},
					},
				},
			}),
			result: func(us ir.BackendObjectIR) *ir.EndpointsForBackend {
				// output
				emd := ir.EndpointWithMd{
					LbEndpoint: &endpointv3.LbEndpoint{
						LoadBalancingWeight: wrapperspb.UInt32(1),
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
							Endpoint: &endpointv3.Endpoint{
								Address: &envoy_config_core_v3.Address{
									Address: &envoy_config_core_v3.Address_SocketAddress{
										SocketAddress: &envoy_config_core_v3.SocketAddress{
											Address: "1.2.3.4",
											PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
												PortValue: 8080,
											},
										},
									},
								},
							},
						},
					},
					EndpointMd: ir.EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone",
							"label":                    "value",
						},
					},
				}
				result := ir.NewEndpointsForBackend(us)
				result.Add(ir.PodLocality{
					Region: "region",
					Zone:   "zone",
				}, emd)
				return result
			},
		},
		{
			name: "deduplication of endpoints across endpointslices",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "ns",
						Labels: map[string]string{
							"app": "test",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						PodIP: "1.2.3.4",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region1",
							corev1.LabelTopologyZone:   "zone1",
						},
					},
				},
				&discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-slice1",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"1.2.3.4"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Pod",
								Name:      "pod1",
								Namespace: "ns",
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name:     ptr.To("http"),
							Port:     ptr.To(int32(8080)),
							Protocol: ptr.To(corev1.ProtocolTCP),
						},
					},
				},
				&discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-slice2",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"1.2.3.4"}, // Duplicate endpoint
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Pod",
								Name:      "pod1",
								Namespace: "ns",
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name:     ptr.To("http"),
							Port:     ptr.To(int32(8080)),
							Protocol: ptr.To(corev1.ProtocolTCP),
						},
					},
				},
			},
			upstream: newBackendObjectIR(ir.BackendObjectIR{
				ObjectSource: ir.ObjectSource{
					Namespace: "ns",
					Name:      "svc",
					Group:     "",
					Kind:      "Service",
				},
				Port: 8080,
				Obj: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "http",
								Port: 8080,
							},
						},
					},
				},
			}),
			result: func(us ir.BackendObjectIR) *ir.EndpointsForBackend {
				// Only one endpoint should be present after deduplication
				emd := ir.EndpointWithMd{
					LbEndpoint: &endpointv3.LbEndpoint{
						LoadBalancingWeight: wrapperspb.UInt32(1),
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
							Endpoint: &endpointv3.Endpoint{
								Address: &envoy_config_core_v3.Address{
									Address: &envoy_config_core_v3.Address_SocketAddress{
										SocketAddress: &envoy_config_core_v3.SocketAddress{
											Address: "1.2.3.4",
											PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
												PortValue: 8080,
											},
										},
									},
								},
							},
						},
					},
					EndpointMd: ir.EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region1",
							corev1.LabelTopologyZone:   "zone1",
							"app":                      "test",
						},
					},
				}
				result := ir.NewEndpointsForBackend(us)
				result.Add(ir.PodLocality{
					Region: "region1",
					Zone:   "zone1",
				}, emd)
				return result
			},
		},
		{
			name: "filter out unready endpoints",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "ns",
						Labels: map[string]string{
							"app": "test",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						PodIP: "1.2.3.4",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region1",
							corev1.LabelTopologyZone:   "zone1",
						},
					},
				},
				&discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-slice-unready",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"1.2.3.4"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(false), // This endpoint is unready
							},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Pod",
								Name:      "pod1",
								Namespace: "ns",
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name:     ptr.To("http"),
							Port:     ptr.To(int32(8080)),
							Protocol: ptr.To(corev1.ProtocolTCP),
						},
					},
				},
			},
			upstream: newBackendObjectIR(ir.BackendObjectIR{
				ObjectSource: ir.ObjectSource{
					Namespace: "ns",
					Name:      "svc",
					Group:     "",
					Kind:      "Service",
				},
				Port: 8080,
				Obj: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "http",
								Port: 8080,
							},
						},
					},
				},
			}),
			result: func(us ir.BackendObjectIR) *ir.EndpointsForBackend {
				// The result should be empty since no ready endpoints are available.
				result := ir.NewEndpointsForBackend(us)
				return result
			},
		},
		{
			name: "multiple ports",
			inputs: []any{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "ns",
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						PodIP: "1.2.3.4",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region1",
							corev1.LabelTopologyZone:   "zone1",
						},
					},
				},
				&discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-slice-unready",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"1.2.3.4"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
							TargetRef: &corev1.ObjectReference{
								Kind:      "Pod",
								Name:      "pod1",
								Namespace: "ns",
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name:     ptr.To("third-port"),
							Port:     ptr.To(int32(3000)),
							Protocol: ptr.To(corev1.ProtocolTCP),
						},
						{
							Name:     ptr.To("first-port"),
							Port:     ptr.To(int32(3000)),
							Protocol: ptr.To(corev1.ProtocolTCP),
						},
						{
							Name:     ptr.To("second-port"),
							Port:     ptr.To(int32(3001)),
							Protocol: ptr.To(corev1.ProtocolTCP),
						},
					},
				},
			},
			upstream: newBackendObjectIR(ir.BackendObjectIR{
				ObjectSource: ir.ObjectSource{
					Namespace: "ns",
					Name:      "svc",
					Group:     "",
					Kind:      "Service",
				},
				Port: 8081,
				Obj: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc",
						Namespace: "ns",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name:       "first-port",
								Port:       8080,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(3000),
							},
							{
								Name:       "second-port",
								Port:       8081,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(3001),
							},
							{
								Name:       "third-port",
								Port:       8082,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(3000),
							},
						},
					},
				},
			}),
			result: func(us ir.BackendObjectIR) *ir.EndpointsForBackend {
				// output
				emd := ir.EndpointWithMd{
					LbEndpoint: &endpointv3.LbEndpoint{
						LoadBalancingWeight: wrapperspb.UInt32(1),
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
							Endpoint: &endpointv3.Endpoint{
								Address: &envoy_config_core_v3.Address{
									Address: &envoy_config_core_v3.Address_SocketAddress{
										SocketAddress: &envoy_config_core_v3.SocketAddress{
											Address: "1.2.3.4",
											PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
												PortValue: 3001,
											},
										},
									},
								},
							},
						},
					},
					EndpointMd: ir.EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region1",
							corev1.LabelTopologyZone:   "zone1",
						},
					},
				}
				result := ir.NewEndpointsForBackend(us)
				result.Add(ir.PodLocality{
					Region: "region1",
					Zone:   "zone1",
				}, emd)
				return result
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			mock := krttest.NewMock(t, tc.inputs)
			nodes := NewNodeMetadataCollection(krttest.GetMockCollection[*corev1.Node](mock))
			pods := NewLocalityPodsCollection(nodes, krttest.GetMockCollection[*corev1.Pod](mock), krtutil.KrtOptions{})
			pods.WaitUntilSynced(context.Background().Done())
			endpointSettings := EndpointsSettings{
				EnableAutoMtls: false,
			}

			// Get the EndpointSlices collection
			endpointSlices := krttest.GetMockCollection[*discoveryv1.EndpointSlice](mock)

			// Initialize the EndpointSlicesByService index
			endpointSlicesByService := krt.NewIndex(endpointSlices, func(es *discoveryv1.EndpointSlice) []types.NamespacedName {
				svcName, ok := es.Labels[discoveryv1.LabelServiceName]
				if !ok {
					return nil
				}
				return []types.NamespacedName{{
					Namespace: es.Namespace,
					Name:      svcName,
				}}
			})

			ei := EndpointsInputs{
				Backends:                krttest.GetMockCollection[ir.BackendObjectIR](mock),
				EndpointSlices:          endpointSlices,
				EndpointSlicesByService: endpointSlicesByService,
				Pods:                    pods,
				EndpointsSettings:       endpointSettings,
			}
			builder := transformK8sEndpoints(ei, nil)

			eps := builder(krt.TestingDummyContext{}, tc.upstream)
			res := tc.result(tc.upstream)
			g.Expect(eps.Equals(*res)).To(BeTrue(), "expected %v, got %v", res, eps)
		})
	}
}
