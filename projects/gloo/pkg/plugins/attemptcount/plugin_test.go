package attemptcount_test

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	types "github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/attemptcount"
)

var _ = Describe("AttemptCount Plugin", func() {
	var (
		acPlugin *Plugin
	)

	BeforeEach(func() {
		acPlugin = NewPlugin()
	})

	It("Allows setting both values independently", func() {
		out := &envoyroute.VirtualHost{}

		err := acPlugin.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				IncludeRequestAttemptCount: &types.BoolValue{
					Value: true,
				},
			},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetIncludeRequestAttemptCount()).To(Equal(true))
		Expect(out.GetIncludeAttemptCountInResponse()).To(Equal(false))

		err = acPlugin.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				IncludeRequestAttemptCount: &types.BoolValue{
					Value: false,
				},
				IncludeAttemptCountInResponse: &types.BoolValue{
					Value: true,
				},
			},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetIncludeRequestAttemptCount()).To(Equal(false))
		Expect(out.GetIncludeAttemptCountInResponse()).To(Equal(true))
	})

	It("still causes both values to default to false", func() {
		out := &envoyroute.VirtualHost{}
		err := acPlugin.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetIncludeRequestAttemptCount()).To(Equal(false))
		Expect(out.GetIncludeAttemptCountInResponse()).To(Equal(false))

		err = acPlugin.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				IncludeRequestAttemptCount:    &types.BoolValue{},
				IncludeAttemptCountInResponse: &types.BoolValue{},
			},
		}, out)

		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetIncludeRequestAttemptCount()).To(Equal(false))
		Expect(out.GetIncludeAttemptCountInResponse()).To(Equal(false))
	})
})
