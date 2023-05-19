package stats

import (
	"context"
	"net/http"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	statsapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Virtual Clusters", func() {

	var (
		ctx          = context.Background()
		plugin       plugins.VirtualHostPlugin
		pluginParams plugins.VirtualHostParams
		inputVh      v1.VirtualHost
		outputVh     envoy_config_route_v3.VirtualHost
		referenceVh  envoy_config_route_v3.VirtualHost
	)

	BeforeEach(func() {
		plugin = NewPlugin()
		plugin.Init(plugins.InitParams{Ctx: ctx})

		pluginParams = plugins.VirtualHostParams{Params: plugins.Params{Ctx: ctx}}
		inputVh = v1.VirtualHost{
			Name:    "my-vh",
			Domains: []string{"a.com", "b.com"},
			Options: &v1.VirtualHostOptions{
				Stats: &statsapi.Stats{
					VirtualClusters: nil,
				},
			},
		}
		outputVh = envoy_config_route_v3.VirtualHost{
			Name:    "my-vh",
			Domains: []string{"a.com", "b.com"},
		}
		referenceVh = envoy_config_route_v3.VirtualHost{
			Name:    "my-vh",
			Domains: []string{"a.com", "b.com"},
		}
	})

	It("does nothing if no virtual clusters are specified", func() {
		err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
		Expect(err).NotTo(HaveOccurred())
		Expect(outputVh).To(Equal(referenceVh))
	})

	getPattern := func(vc *envoy_config_route_v3.VirtualCluster) string {
		return vc.GetHeaders()[0].GetSafeRegexMatch().GetRegex()
	}
	getMethod := func(vc *envoy_config_route_v3.VirtualCluster) string {
		if len(vc.GetHeaders()) < 2 {
			return ""
		}
		return vc.GetHeaders()[1].GetExactMatch()
	}

	It("correctly processes virtual clusters", func() {
		inputVh.Options.Stats.VirtualClusters = []*statsapi.VirtualCluster{
			{Name: "get", Pattern: "/test/.*", Method: "get"},
			{Name: "post", Pattern: "/test/.*", Method: "POST"},
		}
		err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
		Expect(err).NotTo(HaveOccurred())

		Expect(outputVh.VirtualClusters).To(HaveLen(2))

		Expect(outputVh.VirtualClusters[0].Name).To(Equal("get"))
		Expect(getPattern(outputVh.VirtualClusters[0])).To(Equal("/test/.*"))
		Expect(getMethod(outputVh.VirtualClusters[0])).To(Equal(http.MethodGet))

		Expect(outputVh.VirtualClusters[1].Name).To(Equal("post"))
		Expect(getPattern(outputVh.VirtualClusters[1])).To(Equal("/test/.*"))
		Expect(getMethod(outputVh.VirtualClusters[1])).To(Equal(http.MethodPost))

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
		Expect(getPattern(outputVh.VirtualClusters[0])).To(Equal("/test/.*"))
	})

	It("correctly defaults missing method name", func() {
		inputVh.Options.Stats.VirtualClusters = []*statsapi.VirtualCluster{{Name: "test", Pattern: "/test/.*"}}
		err := plugin.ProcessVirtualHost(pluginParams, &inputVh, &outputVh)
		Expect(err).NotTo(HaveOccurred())

		Expect(outputVh.VirtualClusters).To(HaveLen(1))
		Expect(outputVh.VirtualClusters[0].Name).To(Equal("test"))
		Expect(getPattern(outputVh.VirtualClusters[0])).To(Equal("/test/.*"))
		Expect(getMethod(outputVh.VirtualClusters[0])).To(Equal(""))
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
