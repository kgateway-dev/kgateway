package advanced_http_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/advanced_http"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/advanced_http"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("advanced_http plugin", func() {
	var (
		upstream     *v1.Upstream
		upstreamSpec *v1static.UpstreamSpec
	)

	It("should not process upstream if advanced_http config is nil", func() {
		p := NewPlugin()
		err := p.ProcessUpstream(plugins.Params{}, &v1.Upstream{}, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("will err on ProcessUpstream() if advanced_http is configured", func() {
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

		upstreamSpec.Hosts[0].HealthCheckConfig.Path = ""
		upstreamSpec.Hosts[0].HealthCheckConfig.Method = "POST"

		err = p.ProcessUpstream(plugins.Params{}, upstream, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))

		upstreamSpec.Hosts[0].HealthCheckConfig.Path = ""
		upstreamSpec.Hosts[0].HealthCheckConfig.Method = ""
		upstream.HealthChecks = []*core1.HealthCheck{
			{
				HealthChecker: &core1.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &core1.HealthCheck_HttpHealthCheck{
						ResponseAssertions: &advanced_http.ResponseAssertions{},
					},
				},
			},
		}

		err = p.ProcessUpstream(plugins.Params{}, upstream, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
	})

})
