package istio_integration_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/istio_integration"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {
	const (
		serviceNamespace = "serviceNs"
		serviceName      = "serviceName"
		rewrittenHost    = "serviceName.serviceNs"
		upstreamName     = "test-us"
		glooNamespace    = "ns"
	)

	var (
		upstreams v1.UpstreamList
	)

	BeforeEach(func() {
		upstreams = v1.UpstreamList{makeKubeUpstream(upstreamName, glooNamespace, serviceName, serviceNamespace)}
	})

	It("Gets the host from a kube destination", func() {
		destination := &v1.RouteAction_Single{
			Single: &v1.Destination{
				DestinationType: &v1.Destination_Kube{
					Kube: &v1.KubernetesServiceDestination{
						Ref: &core.ResourceRef{
							Namespace: serviceNamespace,
							Name:      serviceName,
						},
					},
				},
			},
		}
		host, err := istio_integration.GetHostFromDestination(destination, upstreams)
		Expect(err).NotTo(HaveOccurred())
		Expect(host).To(Equal(rewrittenHost))
	})
	It("Gets the host from a gloo upstream", func() {
		destination := &v1.RouteAction_Single{
			Single: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: &core.ResourceRef{
						Namespace: glooNamespace,
						Name:      upstreamName,
					},
				},
			},
		}
		host, err := istio_integration.GetHostFromDestination(destination, upstreams)
		Expect(err).NotTo(HaveOccurred())
		Expect(host).To(Equal(rewrittenHost))
	})
})

func makeKubeUpstream(name, namespace, serviceName, serviceNamespace string) *v1.Upstream {
	us := v1.NewUpstream(namespace, name)
	us.UpstreamType = &v1.Upstream_Kube{
		Kube: &kubeplugin.UpstreamSpec{
			ServiceNamespace: serviceNamespace,
			ServiceName:      serviceName,
		},
	}
	return us
}
