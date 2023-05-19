package ratelimit_test

import (
	"context"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/golang/protobuf/ptypes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/test/matchers"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("RateLimit", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		settingsClient gloov1.SettingsClient
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())

		settingsClient = helpers.MustSettingsClient(ctx)
		settings := &gloov1.Settings{
			Metadata: &core.Metadata{
				Name:      "default",
				Namespace: defaults.GlooSystem,
			},
		}
		_, err := settingsClient.Write(settings, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	Run := func(cmd string) {
		err := testutils.Glooctl(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	It("should set timeout", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --request-timeout=1s")
		rlSettings := readRateLimitSettings(ctx, settingsClient)
		Expect(rlSettings.RequestTimeout).To(matchers.MatchProto(ptypes.DurationProto(time.Second)))
	})
	It("should set upstream", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --ratelimit-server-name=test --ratelimit-server-namespace=testns")
		rlSettings := readRateLimitSettings(ctx, settingsClient)
		Expect(rlSettings.RatelimitServerRef.Name).To(Equal("test"))
		Expect(rlSettings.RatelimitServerRef.Namespace).To(Equal("testns"))
	})

	It("should set fail mode deny", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true")
		rlSettings := readRateLimitSettings(ctx, settingsClient)
		Expect(rlSettings.DenyOnFail).To(Equal(true))
	})

	It("should not reset fail mode deny set fail mode deny", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true")
		Run("edit settings --name default --namespace gloo-system ratelimit --request-timeout=1s")
		rlSettings := readRateLimitSettings(ctx, settingsClient)
		Expect(rlSettings.DenyOnFail).To(Equal(true))
	})

	It("should not reset timeout change changing other things", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --request-timeout=1s")
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true")
		rlSettings := readRateLimitSettings(ctx, settingsClient)
		Expect(rlSettings.RequestTimeout).To(matchers.MatchProto(ptypes.DurationProto(time.Second)))
	})

	It("should not set fail mode deny when explicitly set", func() {
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true")
		Run("edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=false")
		rlSettings := readRateLimitSettings(ctx, settingsClient)
		Expect(rlSettings.DenyOnFail).To(Equal(false))
	})

	Context("Interactive tests", func() {

		BeforeEach(func() {
			upstreamClient := helpers.MustUpstreamClient(ctx)
			upstream := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "test",
					Namespace: "test",
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
			ctxCpy := ctx

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
				settings, err := settingsClientCpy.Read(defaults.GlooSystem, "default", clients.ReadOpts{Ctx: ctxCpy})
				Expect(err).NotTo(HaveOccurred())

				Expect(settings.GetRatelimitServer()).ToNot(BeNil())
				rlSettings := *settings.GetRatelimitServer()

				second := ptypes.DurationProto(time.Second)
				expectedSettings := ratelimitpb.Settings{
					DenyOnFail:     true,
					RequestTimeout: second,
					RatelimitServerRef: &core.ResourceRef{
						Name:      "ratelimit",
						Namespace: "gloo-system",
					},
				}

				Expect(rlSettings.RatelimitServerRef).To(matchers.MatchProto(expectedSettings.RatelimitServerRef))
				Expect(rlSettings.RequestTimeout).To(matchers.MatchProto(expectedSettings.RequestTimeout))
				Expect(rlSettings.DenyOnFail).To(Equal(expectedSettings.DenyOnFail))
				Expect(rlSettings.EnableXRatelimitHeaders).To(Equal(expectedSettings.EnableXRatelimitHeaders))
				Expect(rlSettings.RateLimitBeforeAuth).To(Equal(expectedSettings.RateLimitBeforeAuth))
			})
		})

	})

})

func readRateLimitSettings(ctx context.Context, settingsClient gloov1.SettingsClient) ratelimitpb.Settings {
	settings, err := settingsClient.Read(defaults.GlooSystem, "default", clients.ReadOpts{Ctx: ctx})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	ExpectWithOffset(1, settings.GetRatelimitServer()).ToNot(BeNil())
	return *settings.GetRatelimitServer()
}
