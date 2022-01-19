package waf_test

import (
	envoy_config_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	waf "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/waf"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/waf"
)

var _ = Describe("waf plugin", func() {

	It("should not add filter if waf config is nil", func() {
		p := NewPlugin()
		f, err := p.HttpFilters(plugins.Params{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(BeNil())
	})

	It("will err if waf is configured", func() {
		p := NewPlugin()
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Waf: &waf.Settings{},
			},
		}

		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
		Expect(f).To(BeNil())
	})

	It("will err if waf is configured on vhost", func() {
		p := NewPlugin()
		virtualHost := &v1.VirtualHost{
			Name:    "virt1",
			Domains: []string{"*"},
			Options: &v1.VirtualHostOptions{
				Waf: &waf.Settings{},
			},
		}

		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, virtualHost, &envoy_config_route.VirtualHost{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
	})

	It("will err if waf is configured on route", func() {
		p := NewPlugin()
		virtualHost := &v1.Route{
			Name: "route1",
			Options: &v1.RouteOptions{
				Waf: &waf.Settings{},
			},
		}

		err := p.ProcessRoute(plugins.RouteParams{}, virtualHost, &envoy_config_route.Route{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(ErrEnterpriseOnly))
	})
})
