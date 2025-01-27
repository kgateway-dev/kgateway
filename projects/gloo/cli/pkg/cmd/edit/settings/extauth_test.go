package settings_test

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Extauth", func() {
	var (
		settings       *gloov1.Settings
		settingsClient gloov1.SettingsClient
		ctx            context.Context
		cancel         context.CancelFunc
	)
	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())

		settings = testutils.GetTestSettings()
		settingsClient = helpers.MustSettingsClient(ctx)
		_, err := settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	DescribeTable("should edit extauth config",
		func(cmd string, expected *extauthpb.Settings) {

			originalSettings := *settings

			err := testutils.Glooctl(cmd)
			Expect(err).NotTo(HaveOccurred())

			extension := readExtAuthSettings(settingsClient)
			Expect(extension).To(matchers.MatchProto(expected))

			// check that the rest of the settings were not changed.
			settings.Extauth = nil
			settings.Metadata.ResourceVersion = ""
			Expect(settings).To(matchers.MatchProto(&originalSettings))

		},
		Entry("edit name", "edit settings extauth --name default --extauth-server-name test",
			&extauthpb.Settings{
				ExtauthzServerRef: &core.ResourceRef{
					Name:      "test",
					Namespace: "",
				},
			}),
		Entry("edit name", "edit settings extauth --name default --extauth-server-namespace test",
			&extauthpb.Settings{
				ExtauthzServerRef: &core.ResourceRef{
					Name:      "",
					Namespace: "test",
				},
			}),
		Entry("edit name", "edit settings extauth --name default --extauth-server-namespace test --extauth-server-name testname",
			&extauthpb.Settings{
				ExtauthzServerRef: &core.ResourceRef{
					Name:      "testname",
					Namespace: "test",
				},
			}),
	)

	Context("Interactive tests", func() {

		BeforeEach(func() {
			upstreamClient := helpers.MustUpstreamClient(ctx)
			upstream := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "extauth",
					Namespace: "gloo-system",
				},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static_plugin_gloo.UpstreamSpec{
						Hosts: []*static_plugin_gloo.Host{{
							Addr: "test",
							Port: 1234,
						}},
					},
				},
			}
			_, err := upstreamClient.Write(upstream, clients.WriteOpts{OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should enabled auth on route", func() {
			// Assertions are performed in a separate goroutine, so we copy the values to avoid race conditions
			settingsClientCpy := settingsClient

			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("")
				c.ExpectString("name of the resource:")
				c.SendLine("default")
				c.ExpectString("name of the extauth server upstream:")
				c.SendLine("extauth")
				c.ExpectString("namespace of the extauth server upstream:")
				c.SendLine("gloo-system")
				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("edit settings externalauth -i")
				Expect(err).NotTo(HaveOccurred())
				extension := readExtAuthSettings(settingsClientCpy)
				Expect(extension).To(matchers.MatchProto(&extauthpb.Settings{
					ExtauthzServerRef: &core.ResourceRef{
						Name:      "extauth",
						Namespace: "gloo-system",
					},
				}))
			})
		})

	})
})

func readExtAuthSettings(settingsClient gloov1.SettingsClient) *extauthpb.Settings {
	settings, err := settingsClient.Read(defaults.GlooSystem, "default", clients.ReadOpts{})
	Expect(err).NotTo(HaveOccurred())
	return settings.Extauth
}
