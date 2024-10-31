package proxy_syncer_test

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	. "github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func TestTransformsEndpoint(t *testing.T) {
	g := gomega.NewWithT(t)
	us := UpstreamWrapper{
		Inner: &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "name",
				Namespace: "ns",
			},
		},
	}
	efu := NewEndpointsForUpstream(us, nil)
	efu.Add(krtcollections.PodLocality{}, EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "a"}},
					},
				},
			},
		},
		EndpointMd: EndpointMetadata{},
	})

	envoyResources := TransformEndpointToResources(*efu)
	cla := envoyResources.Endpoints.ResourceProto().(*endpointv3.ClusterLoadAssignment)
	locality := cla.Endpoints[0].Locality
	g.Expect(locality).To(gomega.BeNil())
	g.Expect(cla.Endpoints[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetPipe().GetPath()).To(gomega.Equal("a"))
}

func TestTransformsEndpointsWithLocality(t *testing.T) {
	g := gomega.NewWithT(t)
	us := UpstreamWrapper{
		Inner: &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "name",
				Namespace: "ns",
			},
		},
	}
	efu := NewEndpointsForUpstream(us, nil)
	efu.Add(krtcollections.PodLocality{Region: "R1"}, EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "a"}},
					},
				},
			},
		},
		EndpointMd: EndpointMetadata{},
	})
	efu.Add(krtcollections.PodLocality{Region: "R2"}, EndpointWithMd{
		LbEndpoint: &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_Pipe{Pipe: &corev3.Pipe{Path: "a"}},
					},
				},
			},
		},
		EndpointMd: EndpointMetadata{},
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
