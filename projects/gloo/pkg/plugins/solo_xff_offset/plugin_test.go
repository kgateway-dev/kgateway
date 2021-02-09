package solo_xff_offset_test

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/solo_xff_offset"
)

var _ = Describe("solo x-forwarded-for offset plugin", func() {

	It("should not add filter if solo xff offset config is nil", func() {
		p := NewPlugin()
		f, err := p.HttpFilters(plugins.Params{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(BeNil())
	})

	It("will err if solo xff offset is configured", func() {
		p := NewPlugin()
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				LeftmostXffHeader: &wrappers.BoolValue{},
			},
		}

		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
		Expect(f).To(BeNil())
	})

})
