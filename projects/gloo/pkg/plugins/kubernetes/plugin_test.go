package kubernetes

import (
	"context"
	"strings"
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {
	var (
		kube     *fake.Clientset
		params   plugins.Params
		plugin   plugins.Plugin
		upstream *v1.Upstream
		out      *envoy_config_cluster_v3.Cluster
	)
	BeforeEach(func() {
		kube = fake.NewSimpleClientset()
		kubeCoreCache, err := corecache.NewKubeCoreCache(context.Background(), kube)
		Expect(err).To(BeNil())
		plugin = NewPlugin(kube, kubeCoreCache)
		plugin.Init(plugins.InitParams{})
		upstream = &v1.Upstream{
			Metadata: &core.Metadata{
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
		out = &envoy_config_cluster_v3.Cluster{}
	})

	Context("upstreams", func() {

		It("should error upstream with nonexistent service", func() {

			err := plugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
			Expect(strings.Contains(err.Error(), "does not exist in namespace")).To(BeTrue())
		})

		It("should error upstream with nonexistent serviceNamespace", func() {
			// Reset plugin to not watch all namespaces.
			kubeCoreCache, err := corecache.NewKubeCoreCacheWithOptions(context.Background(), kube, time.Duration(1), []string{"ns"})
			Expect(err).To(BeNil())
			plugin = NewPlugin(kube, kubeCoreCache)
			plugin.Init(plugins.InitParams{})
			upstream.UpstreamType = &v1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{
					ServiceName:      "mySvc",
					ServiceNamespace: "ns-fake",
				},
			}
			err = plugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
			Expect(strings.Contains(err.Error(), "invalid ServiceNamespace")).To(BeTrue())
		})

	})

})
