package sanitize_cluster_header

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("sanitize cluster header plugin", func() {
	var (
		p *plugin
	)

	BeforeEach(func() {
		p = NewPlugin()
	})

	It("should not add filter if sanitize cluster header config is nil", func() {
		f, err := p.HttpFilters(plugins.Params{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(BeNil())
	})

	It("will err if sanitize cluster header is configured", func() {
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				SanitizeClusterHeader: &wrappers.BoolValue{},
			},
		}

		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(errEnterpriseOnly))
		Expect(f).To(BeNil())
	})

})
