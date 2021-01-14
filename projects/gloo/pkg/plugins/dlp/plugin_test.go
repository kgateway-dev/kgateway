package dlp_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/dlp"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/dlp"
)

var _ = Describe("dlp plugin", func() {

	It("should not add filter if dlp config is nil", func() {
		p := NewPlugin()
		f, err := p.HttpFilters(plugins.Params{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(BeNil())
	})

	It("will err if dlp is configured", func() {
		p := NewPlugin()
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Dlp: &dlp.FilterConfig{},
			},
		}

		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
		Expect(f).To(BeNil())
	})
})
