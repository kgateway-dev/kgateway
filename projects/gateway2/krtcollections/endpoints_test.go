package krtcollections

import (
	"context"
	"testing"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func TestEndpointsForUpstreamOrderDoesntMatter(t *testing.T) {
	g := gomega.NewWithT(t)

	us := UpstreamWrapper{
		Inner: &gloov1.Upstream{
			Metadata: &core.Metadata{Name: "name", Namespace: "ns"},
			UpstreamType: &gloov1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{
					ServiceName:      "svc",
					ServiceNamespace: "ns",
					ServicePort:      8080,
				},
			},
		},
	}
	// input
	emd1 := EndpointWithMd{
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
		EndpointMd: EndpointMetadata{
			Labels: map[string]string{
				corev1.LabelTopologyRegion: "region",
				corev1.LabelTopologyZone:   "zone",
			},
		},
	}
	emd2 := EndpointWithMd{
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
		EndpointMd: EndpointMetadata{
			Labels: map[string]string{
				corev1.LabelTopologyRegion: "region2",
				corev1.LabelTopologyZone:   "zone2",
			},
		},
	}
	result1 := NewEndpointsForUpstream(us, nil)
	result1.Add(PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)
	result1.Add(PodLocality{
		Region: "region2",
		Zone:   "zone2",
	}, emd2)

	result2 := NewEndpointsForUpstream(us, nil)
	result2.Add(PodLocality{
		Region: "region2",
		Zone:   "zone2",
	}, emd2)
	result2.Add(PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)
	g.Expect(result1.Equals(*result2)).To(BeTrue(), "expected %v, got %v", result1, result2)

	// test with non matching locality
	result3 := NewEndpointsForUpstream(us, nil)
	result3.Add(PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)
	result3.Add(PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd2)
	g.Expect(result1.Equals(*result3)).To(BeFalse(), "not expected %v, got %v", result1, result2)

	// test with non matching labels
	result4 := NewEndpointsForUpstream(us, nil)
	result4.Add(PodLocality{
		Region: "region",
		Zone:   "zone",
	}, emd1)

	emd2.EndpointMd.Labels["extra"] = "label"
	result4.Add(PodLocality{
		Region: "region2",
		Zone:   "zone2",
	}, emd2)
	g.Expect(result1.Equals(*result4)).To(BeFalse(), "not expected %v, got %v", result1, result2)

}

func TestEndpoints(t *testing.T) {
	testCases := []struct {
		name     string
		inputs   []any
		upstream UpstreamWrapper
		result   func(UpstreamWrapper) *EndpointsForUpstream
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
				&corev1.Service{
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
			},
			upstream: UpstreamWrapper{
				Inner: &gloov1.Upstream{
					Metadata: &core.Metadata{Name: "name", Namespace: "ns"},
					UpstreamType: &gloov1.Upstream_Kube{
						Kube: &kubernetes.UpstreamSpec{
							ServiceName:      "svc",
							ServiceNamespace: "ns",
							ServicePort:      8080,
						},
					},
				},
			},
			result: func(us UpstreamWrapper) *EndpointsForUpstream {
				// output
				emd := EndpointWithMd{
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
					EndpointMd: EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone",
						},
					},
				}
				result := NewEndpointsForUpstream(us, nil)
				result.Add(PodLocality{
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
				&corev1.Service{
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
			},
			upstream: UpstreamWrapper{
				Inner: &gloov1.Upstream{
					Metadata: &core.Metadata{Name: "name", Namespace: "ns"},
					UpstreamType: &gloov1.Upstream_Kube{
						Kube: &kubernetes.UpstreamSpec{
							ServiceName:      "svc",
							ServiceNamespace: "ns",
							ServicePort:      8080,
						},
					},
				},
			},
			result: func(us UpstreamWrapper) *EndpointsForUpstream {
				// output
				result := NewEndpointsForUpstream(us, nil)
				result.Add(PodLocality{
					Region: "region",
					Zone:   "zone",
				}, EndpointWithMd{
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
					EndpointMd: EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone",
						},
					},
				})
				result.Add(PodLocality{
					Region: "region",
					Zone:   "zone2",
				}, EndpointWithMd{
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
					EndpointMd: EndpointMetadata{
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
				&corev1.Service{
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
			},
			upstream: UpstreamWrapper{
				Inner: &gloov1.Upstream{
					Metadata: &core.Metadata{Name: "name", Namespace: "ns"},
					UpstreamType: &gloov1.Upstream_Kube{
						Kube: &kubernetes.UpstreamSpec{
							ServiceName:      "svc",
							ServiceNamespace: "ns",
							ServicePort:      8080,
						},
					},
				},
			},
			result: func(us UpstreamWrapper) *EndpointsForUpstream {
				// output
				emd := EndpointWithMd{
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
					EndpointMd: EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region",
							corev1.LabelTopologyZone:   "zone",
							"label":                    "value",
						},
					},
				}
				result := NewEndpointsForUpstream(us, nil)
				result.Add(PodLocality{
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
						Name:      "svc1-slice1",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc1",
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
						Name:      "svc1-slice2",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc1",
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
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
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
			},
			upstream: UpstreamWrapper{
				Inner: &gloov1.Upstream{
					Metadata: &core.Metadata{Name: "name", Namespace: "ns"},
					UpstreamType: &gloov1.Upstream_Kube{
						Kube: &kubernetes.UpstreamSpec{
							ServiceName:      "svc1",
							ServiceNamespace: "ns",
							ServicePort:      8080,
						},
					},
				},
			},
			result: func(us UpstreamWrapper) *EndpointsForUpstream {
				// Only one endpoint should be present after deduplication
				emd := EndpointWithMd{
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
					EndpointMd: EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelTopologyRegion: "region1",
							corev1.LabelTopologyZone:   "zone1",
							"app":                      "test",
						},
					},
				}
				result := NewEndpointsForUpstream(us, nil)
				result.Add(PodLocality{
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
						Name:      "svc1-slice-unready",
						Namespace: "ns",
						Labels: map[string]string{
							"kubernetes.io/service-name": "svc1",
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
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
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
			},
			upstream: UpstreamWrapper{
				Inner: &gloov1.Upstream{
					Metadata: &core.Metadata{Name: "name", Namespace: "ns"},
					UpstreamType: &gloov1.Upstream_Kube{
						Kube: &kubernetes.UpstreamSpec{
							ServiceName:      "svc1",
							ServiceNamespace: "ns",
							ServicePort:      8080,
						},
					},
				},
			},
			result: func(us UpstreamWrapper) *EndpointsForUpstream {
				// The result should be empty since no ready endpoints are available.
				result := NewEndpointsForUpstream(us, nil)
				return result
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			mock := krttest.NewMock(t, tc.inputs)
			nodes := NewNodeMetadataCollection(krttest.GetMockCollection[*corev1.Node](mock))
			pods := NewLocalityPodsCollection(nodes, krttest.GetMockCollection[*corev1.Pod](mock), nil)
			pods.Synced().WaitUntilSynced(context.Background().Done())
			es := EndpointsSettings{
				EnableAutoMtls: false,
			}
			endpointSettings := krt.NewStatic(&es, true)

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
				Upstreams:               krttest.GetMockCollection[UpstreamWrapper](mock),
				EndpointSlices:          endpointSlices,
				EndpointSlicesByService: endpointSlicesByService,
				Pods:                    pods,
				EndpointsSettings:       endpointSettings,
				Services:                krttest.GetMockCollection[*corev1.Service](mock),
			}

			builder := transformK8sEndpoints(context.Background(), ei)

			eps := builder(krt.TestingDummyContext{}, tc.upstream)
			res := tc.result(tc.upstream)
			g.Expect(eps.Equals(*res)).To(BeTrue(), "expected %v, got %v", res, eps)
		})
	}

}
