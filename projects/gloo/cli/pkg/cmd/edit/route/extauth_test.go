package route_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Extauth", func() {
	var (
		vsvc     *gatewayv1.VirtualService
		vsClient gatewayv1.VirtualServiceClient
	)
	BeforeEach(func() {
		helpers.UseMemoryClients()
		// create a settings object
		vsClient = helpers.MustVirtualServiceClient()
		vsvc = &gatewayv1.VirtualService{
			Metadata: core.Metadata{
				Name:      "vs",
				Namespace: "gloo-system",
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Routes: []*gatewayv1.Route{{
					Matcher: &v1.Matcher{
						PathSpecifier: &v1.Matcher_Prefix{Prefix: "/"},
					}}, {
					Matcher: &v1.Matcher{
						PathSpecifier: &v1.Matcher_Prefix{Prefix: "/r"},
					}},
				},
			},
		}
		var err error
		vsvc, err = vsClient.Write(vsvc, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	extAuthExtension := func(index int, metadata core.Metadata) *extauthpb.RouteExtension {
		var extAuthRouteExt extauthpb.RouteExtension
		var err error
		vsv, err := helpers.MustVirtualServiceClient().Read(metadata.Namespace, metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		err = utils.UnmarshalExtension(vsv.VirtualHost.Routes[index].RoutePlugins, constants.ExtAuthExtensionName, &extAuthRouteExt)
		if err != nil {
			if err == utils.NotFoundError {
				return nil
			}
			Expect(err).NotTo(HaveOccurred())
		}
		return &extAuthRouteExt
	}
	Context("Non-interactive tests", func() {

		DescribeTable("should edit extauth config",
			func(cmd string, index int, expected *extauthpb.RouteExtension) {

				err := testutils.Glooctl(cmd)
				Expect(err).NotTo(HaveOccurred())

				extension := extAuthExtension(index, vsvc.Metadata)
				Expect(extension).To(Equal(expected))
			},
			Entry("edit route 0 doesnt impact route one", "edit route externalauth --name vs --namespace gloo-system --index 0 --disable=true",
				1,
				nil),
			Entry("edit route and disable it", "edit route externalauth --name vs --namespace gloo-system --index 1 --disable=true",
				1,
				&extauthpb.RouteExtension{
					Disable: true,
				}),
			Entry("edit route and un-disable it", "edit route externalauth --name vs --namespace gloo-system --index 1 --disable=false",
				1,
				&extauthpb.RouteExtension{
					Disable: false,
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
				Expect(extension).To(Equal(&extauthpb.RouteExtension{
					Disable: true,
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
