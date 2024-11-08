package proxy_syncer_test

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	duration "github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	. "github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTransformsEndpoint(t *testing.T) {
	g := gomega.NewWithT(t)
	us := krtcollections.UpstreamWrapper{
		Inner: &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "name",
				Namespace: "ns",
			},
		},
	}
	efu := krtcollections.NewEndpointsForUpstream(us, nil)
	efu.Add(krtcollections.PodLocality{}, krtcollections.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "a"}},
					},
				},
			},
		},
		EndpointMd: krtcollections.EndpointMetadata{},
	})

	envoyResources := TransformEndpointToResources(*efu)
	cla := envoyResources.Endpoints.ResourceProto().(*endpointv3.ClusterLoadAssignment)
	locality := cla.Endpoints[0].Locality
	g.Expect(locality).To(gomega.BeNil())
	g.Expect(cla.Endpoints[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetPipe().GetPath()).To(gomega.Equal("a"))
}

func TestTransformsEndpointsWithLocality(t *testing.T) {
	g := gomega.NewWithT(t)
	us := krtcollections.UpstreamWrapper{
		Inner: &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "name",
				Namespace: "ns",
			},
		},
	}
	efu := krtcollections.NewEndpointsForUpstream(us, nil)
	efu.Add(krtcollections.PodLocality{Region: "R1"}, krtcollections.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "a"}},
					},
				},
			},
		},
		EndpointMd: krtcollections.EndpointMetadata{},
	})
	efu.Add(krtcollections.PodLocality{Region: "R2"}, krtcollections.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "a"}},
					},
				},
			},
		},
		EndpointMd: krtcollections.EndpointMetadata{},
	})

	envoyResources := TransformEndpointToResources(*efu)
	cla := envoyResources.Endpoints.ResourceProto().(*endpointv3.ClusterLoadAssignment)
	g.Expect(cla.Endpoints).To(gomega.HaveLen(2))
	locality1 := cla.Endpoints[0].Locality
	locality2 := cla.Endpoints[1].Locality
	regions := []string{locality1.Region, locality2.Region}
	g.Expect(regions).To(gomega.ContainElement("R1"))
	g.Expect(regions).To(gomega.ContainElement("R2"))

}

func TestTranslatesDestrulesFailoverPriority(t *testing.T) {
	g := gomega.NewWithT(t)
	us := krtcollections.UpstreamWrapper{
		Inner: &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "name",
				Namespace: "ns",
			},
		},
	}
	efu := krtcollections.NewEndpointsForUpstream(us, nil)
	efu.Add(krtcollections.PodLocality{Region: "R1"}, krtcollections.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "a"}},
					},
				},
			},
		},
		EndpointMd: krtcollections.EndpointMetadata{
			Labels: map[string]string{corev1.LabelTopologyRegion: "R1"},
		},
	})
	efu.Add(krtcollections.PodLocality{Region: "R2"}, krtcollections.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "b"}},
					},
				},
			},
		},
		EndpointMd: krtcollections.EndpointMetadata{
			Labels: map[string]string{corev1.LabelTopologyRegion: "R2"},
		},
	})
	ucc := krtcollections.UniqlyConnectedClient{
		Namespace: "ns",
		Locality:  krtcollections.PodLocality{Region: "R1"},
		Labels:    map[string]string{corev1.LabelTopologyRegion: "R1"},
	}

	destRule := &networkingclient.DestinationRule{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.istio.io/v1alpha3",
			Kind:       "DestinationRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "do-failover",
		},
		Spec: networkingv1alpha3.DestinationRule{
			Host: "reviews.gwtest.svc.cluster.local",
			TrafficPolicy: &networkingv1alpha3.TrafficPolicy{
				OutlierDetection: &networkingv1alpha3.OutlierDetection{
					Consecutive_5XxErrors: &wrappers.UInt32Value{Value: 7},
					Interval:              &duration.Duration{Seconds: 300}, // 5 minutes
					BaseEjectionTime:      &duration.Duration{Seconds: 900}, // 15 minutes
				},
				LoadBalancer: &networkingv1alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingv1alpha3.LocalityLoadBalancerSetting{
						FailoverPriority: []string{
							"topology.kubernetes.io/region",
						},
					},
				},
			},
		},
	}

	uccWithEndpoints := PrioritizeEndpoints(nil, &DestinationRuleWrapper{destRule}, *efu, ucc)
	cla := uccWithEndpoints.Endpoints.ResourceProto().(*endpointv3.ClusterLoadAssignment)
	g.Expect(cla.Endpoints).To(gomega.HaveLen(2))

	remoteLocality := cla.Endpoints[0]
	localLocality := cla.Endpoints[1]
	if remoteLocality.Locality.Region == "R1" {
		remoteLocality = cla.Endpoints[1]
		localLocality = cla.Endpoints[0]
	}
	g.Expect(localLocality.Locality.Region).To(gomega.Equal("R1"))
	g.Expect(remoteLocality.Locality.Region).To(gomega.Equal("R2"))

	g.Expect(localLocality.Priority).To(gomega.Equal(uint32(0)))
	g.Expect(remoteLocality.Priority).To(gomega.Equal(uint32(1)))
}

// similar to TestTranslatesDestrulesFailoverPriority but implicit
func TestTranslatesDestrulesFailover(t *testing.T) {
	g := gomega.NewWithT(t)
	us := krtcollections.UpstreamWrapper{
		Inner: &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "name",
				Namespace: "ns",
			},
		},
	}
	efu := krtcollections.NewEndpointsForUpstream(us, nil)
	efu.Add(krtcollections.PodLocality{Region: "R1"}, krtcollections.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "a"}},
					},
				},
			},
		},
		EndpointMd: krtcollections.EndpointMetadata{
			Labels: map[string]string{corev1.LabelTopologyRegion: "R1"},
		},
	})
	efu.Add(krtcollections.PodLocality{Region: "R2"}, krtcollections.EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "b"}},
					},
				},
			},
		},
		EndpointMd: krtcollections.EndpointMetadata{
			Labels: map[string]string{corev1.LabelTopologyRegion: "R2"},
		},
	})
	ucc := krtcollections.UniqlyConnectedClient{
		Namespace: "ns",
		Locality:  krtcollections.PodLocality{Region: "R1"},
		Labels:    map[string]string{corev1.LabelTopologyRegion: "R1"},
	}

	destRule := &networkingclient.DestinationRule{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.istio.io/v1alpha3",
			Kind:       "DestinationRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "do-failover",
		},
		Spec: networkingv1alpha3.DestinationRule{
			Host: "reviews.gwtest.svc.cluster.local",
			TrafficPolicy: &networkingv1alpha3.TrafficPolicy{
				OutlierDetection: &networkingv1alpha3.OutlierDetection{
					Consecutive_5XxErrors: &wrappers.UInt32Value{Value: 7},
					Interval:              &duration.Duration{Seconds: 300}, // 5 minutes
					BaseEjectionTime:      &duration.Duration{Seconds: 900}, // 15 minutes
				},
				LoadBalancer: &networkingv1alpha3.LoadBalancerSettings{
					LocalityLbSetting: &networkingv1alpha3.LocalityLoadBalancerSetting{},
				},
			},
		},
	}

	uccWithEndpoints := PrioritizeEndpoints(nil, &DestinationRuleWrapper{destRule}, *efu, ucc)
	cla := uccWithEndpoints.Endpoints.ResourceProto().(*endpointv3.ClusterLoadAssignment)
	g.Expect(cla.Endpoints).To(gomega.HaveLen(2))

	remoteLocality := cla.Endpoints[0]
	localLocality := cla.Endpoints[1]
	if remoteLocality.Locality.Region == "R1" {
		remoteLocality = cla.Endpoints[1]
		localLocality = cla.Endpoints[0]
	}
	g.Expect(localLocality.Locality.Region).To(gomega.Equal("R1"))
	g.Expect(remoteLocality.Locality.Region).To(gomega.Equal("R2"))

	g.Expect(localLocality.Priority).To(gomega.Equal(uint32(0)))
	g.Expect(remoteLocality.Priority).To(gomega.Equal(uint32(1)))
}
