package kubernetes

import (
	"context"
	"strings"

	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {
	var (
		params   plugins.Params
		plugin   plugins.Plugin
		upstream *v1.Upstream
		out      *envoycluster.Cluster
	)
	BeforeEach(func() {
		kube := fake.NewSimpleClientset()
		kubeCoreCache, err := corecache.NewKubeCoreCache(context.Background(), kube)
		Expect(err).To(BeNil())
		plugin = NewPlugin(kube, kubeCoreCache)
		plugin.Init(plugins.InitParams{})
		upstream = &v1.Upstream{
			Metadata: core.Metadata{
				Name:      "myUpstream",
				Namespace: "ns",
			},
			UpstreamType: &v1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{
					ServiceName:      "mySvc",
					ServiceNamespace: "ns",
				},
			},
		}
		out = &envoycluster.Cluster{}
	})

	Context("upstreams", func() {

		It("should error upstream with nonexistent service", func() {
			err := plugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
			Expect(strings.Contains(err.Error(), "does not exist in namespace")).To(BeTrue())
		})

	})

})
