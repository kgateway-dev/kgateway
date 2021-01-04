package failover

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("failover plugin", func() {
	var (
		p *plugin
	)

	BeforeEach(func() {
		p = new(plugin)
	})

	It("should not process endpoints if failover config is nil", func() {
		err := p.ProcessEndpoints(plugins.Params{}, &v1.Upstream{}, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not process upstream if failover config is nil", func() {
		err := p.ProcessUpstream(plugins.Params{}, &v1.Upstream{}, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("will err if failover is configured on process upstream", func() {
		err := p.ProcessUpstream(plugins.Params{}, &v1.Upstream{Failover: &v1.Failover{}}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(errEnterpriseOnly))
	})

	It("will err if failover is configured on process endpoint", func() {
		err := p.ProcessEndpoints(plugins.Params{}, &v1.Upstream{Failover: &v1.Failover{}}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(errEnterpriseOnly))
	})

})
