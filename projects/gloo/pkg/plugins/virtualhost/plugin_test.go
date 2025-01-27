package virtualhost_test

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/virtualhost"
)

var _ = Describe("AttemptCount Plugin", func() {
	var (
		acPlugin plugins.VirtualHostPlugin
	)

	BeforeEach(func() {
		acPlugin = NewPlugin()
	})

	It("allows setting both values independently", func() {
		out := &envoy_config_route_v3.VirtualHost{}

		err := acPlugin.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				IncludeRequestAttemptCount: &wrappers.BoolValue{
					Value: true,
				},
			},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetIncludeRequestAttemptCount()).To(BeTrue())
		Expect(out.GetIncludeAttemptCountInResponse()).To(BeFalse())

		err = acPlugin.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				IncludeRequestAttemptCount: &wrappers.BoolValue{
					Value: false,
				},
				IncludeAttemptCountInResponse: &wrappers.BoolValue{
					Value: true,
				},
			},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetIncludeRequestAttemptCount()).To(BeFalse())
		Expect(out.GetIncludeAttemptCountInResponse()).To(BeTrue())
	})

	It("still causes both values to default to false", func() {
		out := &envoy_config_route_v3.VirtualHost{}
		err := acPlugin.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetIncludeRequestAttemptCount()).To(BeFalse())
		Expect(out.GetIncludeAttemptCountInResponse()).To(BeFalse())

		err = acPlugin.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				IncludeRequestAttemptCount:    &wrappers.BoolValue{},
				IncludeAttemptCountInResponse: &wrappers.BoolValue{},
			},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetIncludeRequestAttemptCount()).To(BeFalse())
		Expect(out.GetIncludeAttemptCountInResponse()).To(BeFalse())
	})
})
