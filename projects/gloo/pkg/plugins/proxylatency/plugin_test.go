package proxylatency

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/proxylatency"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("proxylatency plugin", func() {
	var (
		p *plugin
	)

	BeforeEach(func() {
		p = NewPlugin()
	})

	It("should not add filter if proxylatency config is nil", func() {
		f, err := p.HttpFilters(plugins.Params{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(BeNil())
	})

	It("will err if proxylatency is configured", func() {

		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				ProxyLatency: &proxylatency.ProxyLatency{},
			},
		}

		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(errEnterpriseOnly))
		Expect(f).To(BeNil())
	})
})
