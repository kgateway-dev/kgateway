package upstream_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Root", func() {
	var (
		upstream *gloov1.Upstream
		upClient gloov1.UpstreamClient
	)
	BeforeEach(func() {
		helpers.UseMemoryClients()
		// create a settings object
		upClient = helpers.MustUpstreamClient()
		upstream = &gloov1.Upstream{
			Metadata: core.Metadata{
				Name:      "up",
				Namespace: "gloo-system",
			},
		}
	})

	RefreshUpstream := func() {
		var err error
		upstream, err = upClient.Write(upstream, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
	}

	JustBeforeEach(func() {
		RefreshUpstream()
	})

	Glooctl := func(cmd string) {
		err := testutils.Glooctl(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		upstream, err = upClient.Read(upstream.Metadata.Namespace, upstream.Metadata.Name, clients.ReadOpts{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	Context("non interactive", func() {

		It("should update ssl config", func() {
			Glooctl("edit upstream --name up --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
			ref := upstream.GetSslConfig().GetSecretRef()
			Expect(ref).NotTo(BeNil())
			Expect(ref.Name).To(Equal("sslname"))
			Expect(ref.Namespace).To(Equal("sslnamespace"))
		})

		It("should update sni config", func() {
			Glooctl("edit upstream --name up --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace --ssl-sni sniname")
			sslconfig := upstream.GetSslConfig()
			Expect(sslconfig).NotTo(BeNil())
			Expect(sslconfig.Sni).To(Equal("sniname"))
		})

		Context("with existing config", func() {

			BeforeEach(func() {
				upstream.SslConfig = &gloov1.UpstreamSslConfig{
					Sni: "somesni",
				}
			})

			It("should remove ssl config", func() {
				Glooctl("edit upstream --name up --namespace gloo-system --ssl-remove")
				sslconfig := upstream.GetSslConfig()
				Expect(sslconfig).To(BeNil())
			})

			It("should update existing ssl config with resource version", func() {
				Glooctl("edit upstream --resource-version " + upstream.Metadata.ResourceVersion + " --name up --namespace gloo-system --ssl-secret-name sslname --ssl-secret-namespace sslnamespace")
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
				oldResourceVersion := upstream.Metadata.ResourceVersion
				// mutate the upstream
				upstream.Metadata.Annotations = map[string]string{"test": "test"}
				RefreshUpstream()

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
				sslconfig := upstream.GetSslConfig()
				ref := sslconfig.GetSecretRef()

				Expect(ref.Name).To(Equal("sslname"))
				Expect(ref.Namespace).To(Equal("sslnamespace"))
				Expect(sslconfig.Sni).To(Equal("somesni"))
			})
		})
	})
})
