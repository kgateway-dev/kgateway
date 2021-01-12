package proxylatency_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/proxylatency"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/proxylatency"
)

var _ = Describe("proxylatency plugin", func() {

	It("should not add filter if proxylatency config is nil", func() {
		p := NewPlugin()
		f, err := p.HttpFilters(plugins.Params{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(BeNil())
	})

	It("will err if proxylatency is configured", func() {
		p := NewPlugin()
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				ProxyLatency: &proxylatency.ProxyLatency{},
			},
		}

		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
		Expect(f).To(BeNil())
	})
})
