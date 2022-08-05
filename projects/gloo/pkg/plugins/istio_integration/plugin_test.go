package istio_integration_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/istio_integration"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
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
		usClient     v1.UpstreamClient
		kubeUpstream *v1.Upstream
		cancel       context.CancelFunc
		ctx          context.Context
	)
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		resourceClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
		usClient, err := resourceClientFactory.NewResourceClient(ctx, factory.NewResourceClientParams{ResourceType: &v1.Upstream{}})
		Expect(err).NotTo(HaveOccurred())
		kubeUpstream = makeKubeUpstream(upstreamName, glooNamespace, serviceName, serviceNamespace)
		usClient.Write(kubeUpstream, clients.WriteOpts{})
	})
	AfterEach(func() { cancel() })
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
		host, err := istio_integration.GetHostFromDestination(destination, usClient)
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
