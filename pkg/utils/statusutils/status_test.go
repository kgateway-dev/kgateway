package statusutils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/statusutils"
	"github.com/solo-io/gloo/projects/gloo/api/external/solo/ratelimit"
	ratelimitpkg "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Status", func() {

	var (
		statusClientRed, statusClientGreen resources.StatusClient
	)

	BeforeEach(func() {
		statusClientRed = statusutils.GetStatusClientForNamespace("red")
		statusClientGreen = statusutils.GetStatusClientForNamespace("green")
	})

	Context("single status api (deprecated)", func() {

		It("works with RateLimitConfig (api)", func() {
			rateLimitConfig := &ratelimit.RateLimitConfig{}

			newStatus := &core.Status{
				State: core.Status_Accepted,
			}

			// we should not panic
			statusClientRed.SetStatus(rateLimitConfig, newStatus)

			// we should not panic and we should get out what we put in
			statusRed := statusClientRed.GetStatus(rateLimitConfig)
			Expect(statusRed).To(Equal(newStatus))

			// we should not panic and we should get out what we put in
			statusGreen := statusClientGreen.GetStatus(rateLimitConfig)
			Expect(statusGreen).To(Equal(newStatus))
		})

		It("works with RateLimitConfig (pkg)", func() {
			rateLimitConfig := &ratelimitpkg.RateLimitConfig{}

			newStatus := &core.Status{
				State: core.Status_Accepted,
			}

			// we should not panic
			statusClientRed.SetStatus(rateLimitConfig, newStatus)

			// we should not panic and we should get out what we put in
			statusRed := statusClientRed.GetStatus(rateLimitConfig)
			Expect(statusRed).To(Equal(newStatus))

			// we should not panic and we should get out what we put in
			statusGreen := statusClientGreen.GetStatus(rateLimitConfig)
			Expect(statusGreen).To(Equal(newStatus))
		})

	})

	Context("namespaced statuses api", func() {

		It("works with Upstream", func() {
			upstream := &v1.Upstream{}

			newStatus := &core.Status{
				State: core.Status_Accepted,
			}

			statusClientRed.SetStatus(upstream, newStatus)

			// we should get out what we put in
			statusRed := statusClientRed.GetStatus(upstream)
			Expect(statusRed).To(Equal(newStatus))

			// we should get nil, since the status is stored under a different key
			statusGreen := statusClientGreen.GetStatus(upstream)
			Expect(statusGreen).To(BeNil())
		})
	})

})
