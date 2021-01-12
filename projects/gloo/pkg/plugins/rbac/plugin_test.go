package rbac_test

import (
	envoy_config_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/rbac"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/rbac"
)

var _ = Describe("rbac plugin", func() {

	It("should not add filter if rbac config is nil", func() {
		p := NewPlugin()
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{}, &envoy_config_route.VirtualHost{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("will err if rbac is configured on vhost", func() {
		p := NewPlugin()
		virtualHost := &v1.VirtualHost{
			Name:    "virt1",
			Domains: []string{"*"},
			Options: &v1.VirtualHostOptions{
				Rbac: &rbac.ExtensionSettings{},
			},
		}

		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, virtualHost, &envoy_config_route.VirtualHost{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
	})

	It("will err if rbac is configured on route", func() {
		p := NewPlugin()
		virtualHost := &v1.Route{
			Name: "route1",
			Options: &v1.RouteOptions{
				Rbac: &rbac.ExtensionSettings{},
			},
		}

		err := p.ProcessRoute(plugins.RouteParams{}, virtualHost, &envoy_config_route.Route{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
	})

})
