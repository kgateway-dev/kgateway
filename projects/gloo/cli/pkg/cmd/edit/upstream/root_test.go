package upstream_test

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Root", func() {

	var (
		upClient gloov1.UpstreamClient
		upRef    *core.ResourceRef
		ctx      context.Context
		cancel   context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
		upClient = helpers.MustUpstreamClient(ctx)
		upRef = &core.ResourceRef{
			Namespace: defaults.GlooSystem,
			Name:      "up",
		}

		// Write a basic Upstream so we have something to edit
		_, err := upClient.Write(&gloov1.Upstream{
			Metadata: &core.Metadata{
				Namespace: upRef.GetNamespace(),
				Name:      upRef.GetName(),
			},
		}, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
	})

	Context("non interactive", func() {

		It("should update ssl config", func() {
			Glooctl("edit upstream --name up --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
			upstream := ReadUpstream(upClient, upRef)

			ref := upstream.GetSslConfig().GetSecretRef()
			Expect(ref).NotTo(BeNil())
			Expect(ref.Name).To(Equal("sslname"))
			Expect(ref.Namespace).To(Equal("sslnamespace"))
		})

		It("should update sni config", func() {
			Glooctl("edit upstream --name up --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace --ssl-sni sniname")
			upstream := ReadUpstream(upClient, upRef)

			sslconfig := upstream.GetSslConfig()
			Expect(sslconfig).NotTo(BeNil())
			Expect(sslconfig.Sni).To(Equal("sniname"))
		})

		Context("with existing config", func() {

			BeforeEach(func() {
				upstream := ReadUpstream(upClient, upRef)
				upstream.SslConfig = &ssl.UpstreamSslConfig{
					Sni: "somesni",
				}
				OverwriteUpstream(upClient, upstream)
			})

			It("should remove ssl config", func() {
				Glooctl("edit upstream --name up --namespace gloo-system --ssl-remove")
				upstream := ReadUpstream(upClient, upRef)

				sslconfig := upstream.GetSslConfig()
				Expect(sslconfig).To(BeNil())
			})

			It("should update existing ssl config with resource version", func() {
				upstream := ReadUpstream(upClient, upRef)
				Glooctl("edit upstream --resource-version " + upstream.Metadata.ResourceVersion + " --name up --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
				upstream = ReadUpstream(upClient, upRef)

				sslconfig := upstream.GetSslConfig()
				ref := sslconfig.GetSecretRef()
				Expect(ref).NotTo(BeNil())
				Expect(ref.Name).To(Equal("sslname"))
				Expect(ref.Namespace).To(Equal("sslnamespace"))
				Expect(sslconfig.Sni).To(Equal("somesni"))
			})

		})

		Context("Errors", func() {

			It("should not update with out of date resource version", func() {
				upstream := ReadUpstream(upClient, upRef)
				oldResourceVersion := upstream.Metadata.ResourceVersion
				// mutate the upstream
				upstream.Metadata.Annotations = map[string]string{"test": "test"}
				OverwriteUpstream(upClient, upstream)

				err := testutils.Glooctl("edit upstream --resource-version " + oldResourceVersion + " --name up --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("conflict - resource version does not match"))
			})

			It("should not create a secret ref with just a name", func() {
				err := testutils.Glooctl("edit upstream --name up --namespace gloo-system --ssl-secret-name sslname")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("both --ssl-secret-name and --ssl-secret-namespace must be provided"))
			})

			It("should not create a secret ref with just a namespace", func() {
				err := testutils.Glooctl("edit upstream --name up --namespace gloo-system --ssl-secret-namespace sslnamespace")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("both --ssl-secret-name and --ssl-secret-namespace must be provided"))
			})

			It("should fail on bad upstream", func() {
				err := testutils.Glooctl("edit upstream --name notup --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Error reading upstream: gloo-system.notup does not exist"))
			})
		})
	})

	Context("interactive", func() {

		It("should enabled ssl on upstream", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("yes")
				c.ExpectString("name of the resource:")
				c.SendLine("up")
				c.ExpectString("name of the ssl secret for this upstream:")
				c.SendLine("sslname")
				c.ExpectString("namespace of the ssl secret for this upstream:")
				c.SendLine("sslnamespace")
				c.ExpectString("SNI value to provide when contacting this upstream:")
				c.SendLine("somesni")
				c.ExpectEOF()
			}, func() {
				Glooctl("edit upstream -i")
				upstream := ReadUpstream(helpers.MustUpstreamClient(ctx), &core.ResourceRef{
					Namespace: defaults.GlooSystem,
					Name:      "up",
				})

				sslconfig := upstream.GetSslConfig()
				ref := sslconfig.GetSecretRef()

				Expect(ref.Name).To(Equal("sslname"))
				Expect(ref.Namespace).To(Equal("sslnamespace"))
				Expect(sslconfig.Sni).To(Equal("somesni"))
			})
		})
	})
})

func OverwriteUpstream(client gloov1.UpstreamClient, us *gloov1.Upstream) {
	_, err := client.Write(us, clients.WriteOpts{OverwriteExisting: true})
	Expect(err).NotTo(HaveOccurred())
}

func ReadUpstream(client gloov1.UpstreamClient, ref *core.ResourceRef) *gloov1.Upstream {
	us, err := client.Read(ref.GetNamespace(), ref.GetName(), clients.ReadOpts{})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	return us
}

func Glooctl(cmd string) {
	err := testutils.Glooctl(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
