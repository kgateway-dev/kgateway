package ratelimit_test

import (
	"time"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Ratelimit", func() {

	var (
		settings       *gloov1.Settings
		rlSettings     ratelimitpb.Settings
		settingsClient gloov1.SettingsClient
	)
	BeforeEach(func() {
		helpers.UseMemoryClients()
		// create a settings object
		settingsClient = helpers.MustSettingsClient()

		settings = &gloov1.Settings{
			Metadata: core.Metadata{
				Name:      "default",
				Namespace: "gloo-system",
			},
		}
		rlSettings = ratelimitpb.Settings{}
		var err error
		settings, err = settingsClient.Write(settings, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	ReadSettings := func() {
		var err error
		settings, err = settingsClient.Read(settings.Metadata.Namespace, settings.Metadata.Name, clients.ReadOpts{})
		ExpectWithOffset(2, err).NotTo(HaveOccurred())

		rlSettings = ratelimitpb.Settings{}
		err = utils.UnmarshalExtension(settings, constants.RateLimitExtensionName, &rlSettings)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())
	}

	Run := func(cmd string) {
		err := testutils.Glooctl(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ReadSettings()
	}

	It("should set timeout", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --request-timeout=1s")
		Expect(*rlSettings.RequestTimeout).To(Equal(time.Second))
	})
	It("should set upstream", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --ratelimit-server-name=test --ratelimit-server-namespace=testns")
		Expect(rlSettings.RatelimitServerRef.Name).To(Equal("test"))
		Expect(rlSettings.RatelimitServerRef.Namespace).To(Equal("testns"))
	})

	It("should set fail mode deny", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true")
		Expect(rlSettings.DenyOnFail).To(Equal(true))
	})

	It("should not reset fail mode deny set fail mode deny", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true")
		Run("edit settings --name default --namespace gloo-system ratelimit --request-timeout=1s")
		Expect(rlSettings.DenyOnFail).To(Equal(true))
	})

	It("should not reset timeout change changing other things", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --request-timeout=1s")
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true")
		Expect(*rlSettings.RequestTimeout).To(Equal(time.Second))
	})

	It("should not set fail mode deny when explicitly set", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true")
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=false")
		Expect(rlSettings.DenyOnFail).To(Equal(false))
	})

	Context("Interactive tests", func() {

		BeforeEach(func() {
			upstreamClient := helpers.MustUpstreamClient()
			upstream := &gloov1.Upstream{
				Metadata: core.Metadata{
					Name:      "test",
					Namespace: "test",
				},
				UpstreamSpec: &gloov1.UpstreamSpec{
					UpstreamType: &gloov1.UpstreamSpec_Static{
						Static: &static_plugin_gloo.UpstreamSpec{
							Hosts: []*static_plugin_gloo.Host{{
								Addr: "test",
								Port: 1234,
							}},
						},
					},
				},
			}
			_, err := upstreamClient.Write(upstream, clients.WriteOpts{OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should enabled auth on route", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("")
				c.ExpectString("name of the resource:")
				c.SendLine("default")
				c.ExpectString("name of the ratelimit server upstream:")
				c.SendLine("ratelimit")
				c.ExpectString("namespace of the ratelimit server upstream:")
				c.SendLine("gloo-system")
				c.ExpectString("the timeout for a request:")
				c.SendLine("1s")
				c.ExpectString("enable failure mode deny: ")
				c.SendLine("y")
				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("edit settings ratelimit -i")
				Expect(err).NotTo(HaveOccurred())
				ReadSettings()
				second := time.Second
				expectedSettings := ratelimitpb.Settings{
					DenyOnFail:     true,
					RequestTimeout: &second,
					RatelimitServerRef: &core.ResourceRef{
						Name:      "ratelimit",
						Namespace: "gloo-system",
					},
				}
				Expect(rlSettings).To(BeEquivalentTo(expectedSettings))
			})
		})

	})

})
