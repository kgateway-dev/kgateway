package install_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"time"
)

var _ = Describe("Gateway", func() {
	It("should install the gloo gateway", func() {
		err := testutils.Glooctl("install gateway --release 0.6.14")
		Expect(err).NotTo(HaveOccurred())

		// when we see that discovery has created an upstream for gateway-proxy, we're good
		var us *v1.Upstream
		Eventually(func() (*v1.Upstream, error){
			u, err := helpers.MustUpstreamClient().Read("gloo-system", "gloo-system-gateway-proxy-8080", clients.ReadOpts{})
			us = u
			return us, err
		}, time.Minute).Should(Not(BeNil()))
		Expect(us.Metadata.Name).To(Equal("gloo-system-gateway-proxy-8080"))
	})
})
