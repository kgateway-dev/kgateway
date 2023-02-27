package ratelimit_test

import (
	"context"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmdutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("CustomEnvoyConfig", func() {
	var (
		vsvc     *gatewayv1.VirtualService
		vsClient gatewayv1.VirtualServiceClient
		ctx      context.Context
		cancel   context.CancelFunc
	)
	BeforeEach(func() {
		helpers.UseMemoryClients()
		// create a settings object
		ctx, cancel = context.WithCancel(context.Background())
		vsClient = helpers.MustVirtualServiceClient(ctx)
		vsvc = &gatewayv1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "vs",
				Namespace: "gloo-system",
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Routes: []*gatewayv1.Route{{}},
			},
		}

		var err error
		vsvc, err = vsClient.Write(vsvc, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	rateLimitExtension := func(index int) *ratelimitpb.RateLimitRouteExtension {
		var err error
		vsvc, err = vsClient.Read(vsvc.Metadata.Namespace, vsvc.Metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		return vsvc.VirtualHost.Routes[index].Options.GetRatelimit()
	}

	It("should edit route", func() {

		cmdutils.EditFileForTest = func(prefix, suffix string, r io.Reader) ([]byte, string, error) {
			b := `
include_vh_rate_limits: true
rate_limits:
- actions:
  - source_cluster: {}`
			return []byte(b), "", nil
		}

		err := testutils.Glooctl("edit route --name vs --namespace gloo-system --index 0 ratelimit client-config")
		Expect(err).NotTo(HaveOccurred())

		ext := rateLimitExtension(0)
		Expect(ext.RateLimits).To(HaveLen(1))
		Expect(ext.RateLimits[0].Actions).To(HaveLen(1))
		Expect(ext.IncludeVhRateLimits).To(Equal(true))
	})

})
