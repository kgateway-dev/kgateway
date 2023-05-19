package virtualservice_test

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Root", func() {
	var (
		vs       *gatewayv1.VirtualService
		vsClient gatewayv1.VirtualServiceClient
		ctx      context.Context
		cancel   context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())

		vsClient = helpers.MustVirtualServiceClient(ctx)
		vs = &gatewayv1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "vs",
				Namespace: defaults.GlooSystem,
			},
		}
	})

	AfterEach(func() {
		cancel()
	})

	RefreshVirtualHost := func() {
		var err error
		vs, err = vsClient.Write(vs, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
	}

	JustBeforeEach(func() {
		RefreshVirtualHost()
	})

	Glooctl := func(cmd string) {
		err := testutils.Glooctl(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		vs, err = vsClient.Read(vs.Metadata.Namespace, vs.Metadata.Name, clients.ReadOpts{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	Context("non interactive", func() {

		It("should update ssl config", func() {
			Glooctl("edit virtualservice --name vs --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
			ref := vs.GetSslConfig().GetSecretRef()
			Expect(ref).NotTo(BeNil())
			Expect(ref.Name).To(Equal("sslname"))
			Expect(ref.Namespace).To(Equal("sslnamespace"))
		})

		It("should update sni config", func() {
			Glooctl("edit virtualservice --name vs --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace --ssl-sni-domains sniname")
			sslconfig := vs.GetSslConfig()
			Expect(sslconfig).NotTo(BeNil())
			Expect(sslconfig.SniDomains).To(Equal([]string{"sniname"}))
		})

		Context("with existing config", func() {

			BeforeEach(func() {
				vs.SslConfig = &ssl.SslConfig{
					SniDomains: []string{"somesni"},
				}
			})

			It("should remove ssl config", func() {
				Glooctl("edit virtualservice --name vs --namespace gloo-system --ssl-remove")
				sslconfig := vs.GetSslConfig()
				Expect(sslconfig).To(BeNil())
			})

			It("should update existing ssl config with resource version", func() {
				Glooctl("edit virtualservice --resource-version " + vs.Metadata.ResourceVersion + " --name vs --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
				sslconfig := vs.GetSslConfig()
				ref := sslconfig.GetSecretRef()
				Expect(ref).NotTo(BeNil())
				Expect(ref.Name).To(Equal("sslname"))
				Expect(ref.Namespace).To(Equal("sslnamespace"))
				Expect(sslconfig.SniDomains).To(Equal([]string{"somesni"}))
			})
		})

		Context("Errors", func() {

			It("should not update with out of date resource version", func() {
				oldResourceVersion := vs.Metadata.ResourceVersion
				// mutate the vs
				vs.Metadata.Annotations = map[string]string{"test": "test"}
				RefreshVirtualHost()

				err := testutils.Glooctl("edit virtualservice --resource-version " + oldResourceVersion + " --name vs --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
				Expect(err).To(HaveOccurred())
			})

			It("should fail on bad virtual service", func() {

				err := testutils.Glooctl("edit virtualservice --name notvs --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Error reading virtual service: gloo-system.notvs does not exist"))
			})
		})
	})

	Context("interactive", func() {

		It("should enabled ssl on virtual service", func() {
			// Assertions are performed in a separate goroutine, so we copy the values to avoid race conditions
			vsClientCpy := vsClient
			ctxCpy := ctx

			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("yes")
				c.ExpectString("name of the resource:")
				c.SendLine("vs")
				c.ExpectString("name of the ssl secret for this virtual service:")
				c.SendLine("sslname")
				c.ExpectString("namespace of the ssl secret for this virtual service:")
				c.SendLine("sslnamespace")
				c.ExpectString("SNI domains for this virtual service:")
				c.SendLine("somesni")
				c.ExpectString("SNI domains for this virtual service:")
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("edit virtualservice -i")
				Expect(err).NotTo(HaveOccurred())
				vs, err := vsClientCpy.Read(defaults.GlooSystem, "vs", clients.ReadOpts{Ctx: ctxCpy})
				Expect(err).NotTo(HaveOccurred())

				sslconfig := vs.GetSslConfig()
				ref := sslconfig.GetSecretRef()

				Expect(ref.Name).To(Equal("sslname"))
				Expect(ref.Namespace).To(Equal("sslnamespace"))
				Expect(sslconfig.SniDomains).To(Equal([]string{"somesni"}))
			})
		})
	})
})
