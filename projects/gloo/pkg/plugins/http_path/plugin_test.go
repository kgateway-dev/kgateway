package http_path_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/http_path"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("http_path plugin", func() {
	var (
		upstream     *v1.Upstream
		upstreamSpec *v1static.UpstreamSpec
	)

	It("should not process upstream if http_path config is nil", func() {
		p := NewPlugin()
		err := p.ProcessUpstream(plugins.Params{}, &v1.Upstream{}, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("will err if http_path is configured on process upstream", func() {
		p := NewPlugin()
		upstreamSpec = &v1static.UpstreamSpec{
			Hosts: []*v1static.Host{{
				Addr: "localhost",
				Port: 1234,
				HealthCheckConfig: &v1static.Host_HealthCheckConfig{
					Path: "/foo",
				},
			}},
		}
		upstream = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "extauth-server",
				Namespace: "default",
			},
			UpstreamType: &v1.Upstream_Static{
				Static: upstreamSpec,
			},
		}

		err := p.ProcessUpstream(plugins.Params{}, upstream, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
	})

})
