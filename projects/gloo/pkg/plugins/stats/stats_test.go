package stats

import (
	"context"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	statsapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Virtual Clusters", func() {

	var (
		ctx          = context.Background()
		plugin       *Plugin
		pluginParams = plugins.VirtualHostParams{Params: plugins.Params{Ctx: ctx}}
		inputVh      = v1.VirtualHost{
			Name:    "my-vh",
			Domains: []string{"a.com", "b.com"},
			Options: &v1.VirtualHostOptions{
				Stats: &statsapi.Stats{
					VirtualClusters: nil,
				},
			},
		}
		outputVh = envoyroute.VirtualHost{
			Name:    "my-vh",
			Domains: []string{"a.com", "b.com"},
		}
		referenceVh = envoyroute.VirtualHost{
			Name:    "my-vh",
			Domains: []string{"a.com", "b.com"},
		}
	)

	BeforeEach(func() {
		plugin = NewPlugin()
		Expect(plugin.Init(plugins.InitParams{Ctx: ctx})).NotTo(HaveOccurred())
	})

	It("does nothing if no virtual clusters are specified", func() {
		err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
		Expect(err).NotTo(HaveOccurred())
		Expect(outputVh).To(Equal(referenceVh))
	})

	It("correctly processes virtual clusters", func() {
		inputVh.Options.Stats.VirtualClusters = []*statsapi.VirtualCluster{
			{Name: "get", Pattern: "/test/.*", Method: "GET"},
			{Name: "post", Pattern: "/test/.*", Method: "POST"},
		}
		err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
		Expect(err).NotTo(HaveOccurred())

		Expect(outputVh.VirtualClusters).To(HaveLen(2))

		Expect(outputVh.VirtualClusters[0].Name).To(Equal("get"))
		Expect(outputVh.VirtualClusters[0].Pattern).To(Equal("/test/.*"))
		Expect(outputVh.VirtualClusters[0].Method).To(Equal(envoycore.RequestMethod_GET))

		Expect(outputVh.VirtualClusters[1].Name).To(Equal("post"))
		Expect(outputVh.VirtualClusters[1].Pattern).To(Equal("/test/.*"))
		Expect(outputVh.VirtualClusters[1].Method).To(Equal(envoycore.RequestMethod_POST))

		// Remove virtual clusters and verify that the rest of the resource has not changed
		outputVh.VirtualClusters = nil
		Expect(outputVh).To(Equal(referenceVh))
	})

	It("sanitizes illegal virtual cluster name", func() {
		inputVh.Options.Stats.VirtualClusters = []*statsapi.VirtualCluster{{Name: "not.valid", Pattern: "/test/.*"}}
		err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
		Expect(err).NotTo(HaveOccurred())

		Expect(outputVh.VirtualClusters).To(HaveLen(1))
		Expect(outputVh.VirtualClusters[0].Name).To(Equal("not_valid"))
		Expect(outputVh.VirtualClusters[0].Pattern).To(Equal("/test/.*"))
	})

	It("correctly defaults missing method name", func() {
		inputVh.Options.Stats.VirtualClusters = []*statsapi.VirtualCluster{{Name: "test", Pattern: "/test/.*"}}
		err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
		Expect(err).NotTo(HaveOccurred())

		Expect(outputVh.VirtualClusters).To(HaveLen(1))
		Expect(outputVh.VirtualClusters[0].Name).To(Equal("test"))
		Expect(outputVh.VirtualClusters[0].Pattern).To(Equal("/test/.*"))
		Expect(outputVh.VirtualClusters[0].Method).To(Equal(envoycore.RequestMethod_METHOD_UNSPECIFIED))
	})

	Describe("expected failures", func() {

		It("fails if a virtual cluster name is missing", func() {
			inputVh.Options.Stats.VirtualClusters = []*statsapi.VirtualCluster{{Pattern: "/test/.*"}}
			err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(invalidVirtualClusterErr(missingNameErr, "").Error()))
		})

		It("fails if a virtual cluster pattern is missing", func() {
			inputVh.Options.Stats.VirtualClusters = []*statsapi.VirtualCluster{{Name: "test-vc"}}
			err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(invalidVirtualClusterErr(missingPatternErr, "test-vc").Error()))
		})

		It("fails if an invalid HTTP method is provided", func() {
			misspelledMethod := "DELET"
			inputVh.Options.Stats.VirtualClusters = []*statsapi.VirtualCluster{{
				Name: "test-vc", Pattern: "/test/.*", Method: misspelledMethod}}
			err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(invalidVirtualClusterErr(invalidMethodErr(misspelledMethod), "test-vc").Error()))
		})
	})
})
