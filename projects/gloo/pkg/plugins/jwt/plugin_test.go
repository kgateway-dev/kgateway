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
				Jwt: &jwt.VhostExtension{},
			},
		}

		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, virtualHost, &envoy_config_route.VirtualHost{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
	})

})
