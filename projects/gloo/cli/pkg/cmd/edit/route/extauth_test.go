package route_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	test_matchers "github.com/solo-io/solo-kit/test/matchers"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	extauthpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Extauth", func() {
	var (
		vsvc     *gatewayv1.VirtualService
		vsClient gatewayv1.VirtualServiceClient
		ctx      context.Context
		cancel   context.CancelFunc
	)
	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
		// create a settings object
		vsClient = helpers.MustVirtualServiceClient(ctx)
		vsvc = &gatewayv1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "vs",
				Namespace: "gloo-system",
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Routes: []*gatewayv1.Route{{
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/"},
					}}}, {
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/r"},
					}}},
				},
			},
		}
		var err error
		vsvc, err = vsClient.Write(vsvc, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	extAuthExtension := func(index int, metadata *core.Metadata) *extauthpb.ExtAuthExtension {
		vsv, err := helpers.MustVirtualServiceClient(ctx).Read(metadata.Namespace, metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		return vsv.VirtualHost.Routes[index].GetOptions().GetExtauth()
	}
	Context("Non-interactive tests", func() {

		DescribeTable("should edit extauth config",
			func(cmd string, index int, expected *extauthpb.ExtAuthExtension) {
				err := testutils.Glooctl(cmd)
				Expect(err).NotTo(HaveOccurred())

				extension := extAuthExtension(index, vsvc.Metadata)
				Expect(extension).To(test_matchers.MatchProto(expected))
			},
			Entry("edit route 0 doesnt impact route one", "edit route externalauth --name vs --namespace gloo-system --index 0 --disable=true",
				1,
				nil),
			Entry("edit route and disable it", "edit route externalauth --name vs --namespace gloo-system --index 1 --disable=true",
				1,
				&extauthpb.ExtAuthExtension{
					Spec: &extauthpb.ExtAuthExtension_Disable{Disable: true},
				}),
			Entry("edit route and un-disable it", "edit route externalauth --name vs --namespace gloo-system --index 1 --disable=false",
				1,
				&extauthpb.ExtAuthExtension{
					Spec: nil,
				}),
		)
	})

	Context("Interactive tests", func() {

		It("should enabled auth on route", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("")
				c.ExpectString("name of the resource:")
				c.SendLine("vs")
				c.ExpectString("Choose the route you wish to change:")
				c.PressDown()
				c.SendLine("")
				c.ExpectString("Disable auth on this route?")
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("edit route externalauth -i")
				Expect(err).NotTo(HaveOccurred())
				extension := extAuthExtension(1, vsvc.Metadata)
				Expect(extension).To(test_matchers.MatchProto(&extauthpb.ExtAuthExtension{
					Spec: &extauthpb.ExtAuthExtension_Disable{Disable: true},
				}))
			})
		})

	})

	Context("Errors", func() {
		It("should error with wrong resource version", func() {
			badrv := vsvc.Metadata.ResourceVersion + "a"
			err := testutils.Glooctl("edit route externalauth --name vs --namespace gloo-system --index 1 --disable=true --resource-version=" + badrv)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("conflict - resource version does not match"))
		})

	})
})
