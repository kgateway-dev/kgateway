package dynamic_forward_proxy_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_extensions_filters_http_dynamic_forward_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_forward_proxy/v3"
	"github.com/golang/protobuf/ptypes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/dynamic_forward_proxy"
)

var _ = Describe("enterprise_warning plugin", func() {

	var (
		params     plugins.Params
		initParams plugins.InitParams
		listener   *v1.HttpListener
	)

	BeforeEach(func() {
		params = plugins.Params{}
		initParams = plugins.InitParams{}
		listener = &v1.HttpListener{}
	})

	It("does not configure DFP filter if not needed", func() {
		p := NewPlugin()
		filters, err := p.HttpFilters(params, listener)
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(BeEmpty())
	})

	Context("sane defaults", func() {

		BeforeEach(func() {
			listener.Options = &v1.HttpListenerOptions{
				DynamicForwardProxy: &dynamic_forward_proxy.FilterConfig{}, // pick up system defaults to resolve DNS
			}
		})

		It("uses sane defaults with empty http filter", func() {
			p := NewPlugin()
			err := p.Init(initParams)
			Expect(err).NotTo(HaveOccurred())
			filters, err := p.HttpFilters(params, listener)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(HaveLen(1))

			filterCfg := &envoy_extensions_filters_http_dynamic_forward_proxy_v3.FilterConfig{}
			goTypedConfig := filters[0].HttpFilter.GetTypedConfig()
			err = ptypes.UnmarshalAny(goTypedConfig, filterCfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(filterCfg.GetDnsCacheConfig().GetDnsLookupFamily()).To(Equal(envoy_config_cluster_v3.Cluster_V4_PREFERRED))
		})
	})
})
