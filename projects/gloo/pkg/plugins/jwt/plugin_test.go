package jwt_test

import (
	envoy_config_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/jwt"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/jwt"
)

var _ = Describe("jwt plugin", func() {

	It("should not add filter if jwt config is nil", func() {
		p := NewPlugin()
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{}, &envoy_config_route.VirtualHost{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("will err if jwt is configured", func() {
		p := NewPlugin()
		virtualHost := &v1.VirtualHost{
			Name:    "virt1",
			Domains: []string{"*"},
			Options: &v1.VirtualHostOptions{
				JwtConfig: &v1.VirtualHostOptions_JwtStaged{
					JwtStaged: &jwt.JwtStagedVhostExtension{},
				},
			},
		}

		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, virtualHost, &envoy_config_route.VirtualHost{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
	})

	It("will err if jwt is configured", func() {
		p := NewPlugin()
		route := &v1.Route{
			Name: "route1",
			Options: &v1.RouteOptions{
				JwtConfig: &v1.RouteOptions_JwtStaged{
					JwtStaged: &jwt.JwtStagedRouteExtension{},
				},
			},
		}

		err := p.ProcessRoute(plugins.RouteParams{}, route, &envoy_config_route.Route{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
	})

})
