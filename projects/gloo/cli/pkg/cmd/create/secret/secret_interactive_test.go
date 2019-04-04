package secret

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Secret Interactive Mode", func() {

	const (
		secretNamespace = "gloo-system"
		secretName      = "test-secret"
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	expectMeta := func(meta core.Metadata) {
		Expect(meta.Namespace).To(Equal(secretNamespace))
		Expect(meta.Name).To(Equal(secretName))
	}

	Context("AWS", func() {
		It("should work", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(surveyutils.PromptInteractiveNamespace)
				c.SendLine(secretNamespace)
				c.ExpectString(surveyutils.PromptInteractiveResourceName)
				c.SendLine(secretName)
				c.ExpectString("Enter AWS Access Key ID (leave empty to read credentials from ~/.aws/credentials):")
				c.SendLine("foo")
				c.ExpectString("Enter AWS Secret Key (leave empty to read credentials from ~/.aws/credentials):")
				c.SendLine("bar")
				c.ExpectEOF()
			}, func() {
				opts := &options.Options{
					Top: options.Top{
						Ctx:         context.Background(),
						Interactive: true,
					},
					Metadata: core.Metadata{},
					Create: options.Create{
						InputSecret: options.Secret{
							AwsSecret: options.AwsSecret{
								AccessKey: flagDefaultAwsAccessKey,
								SecretKey: flagDefaultAwsSecretKey,
							},
						},
						DryRun: true,
					},
				}
				cmd := CreateCmd(opts)
				cmd.SetArgs([]string{"aws"})
				err := cmd.Execute()
				Expect(err).NotTo(HaveOccurred())
				expectMeta(opts.Metadata)
				Expect(opts.Create.InputSecret.AwsSecret.AccessKey).To(Equal("foo"))
				Expect(opts.Create.InputSecret.AwsSecret.SecretKey).To(Equal("bar"))
			})
		})
	})

	Context("Azure", func() {
		// TODO: https://github.com/solo-io/gloo/issues/387, see comment below
		PIt("should work", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Please choose a namespace")
				c.SendLine("gloo-system")
				c.ExpectString("name of secret")
				c.SendLine("test-secret")
				c.ExpectString("Enter API key entry (key=value)")
				c.SendLine("foo=bar") // need to find a solution to the idiosyncrasy of slice input
				c.SendLine("gloo=baz")
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				var meta core.Metadata
				var azureSecret options.AzureSecret
				err := AzureSecretArgsInteractive(&meta, &azureSecret)
				Expect(err).NotTo(HaveOccurred())
				expectMeta(meta)
				Expect(azureSecret.ApiKeys.MustMap()).To(BeEquivalentTo(map[string]string{"foo": "bar", "gloo": "baz"}))
			})
		})
	})

	Context("Tls", func() {
		It("should work", func() {
			var (
				rootCa            = "foo"
				privateKey        = "bar"
				certChainFilename = "baz"
			)
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("filename of rootca for secret")
				c.SendLine(rootCa)
				c.ExpectString("filename of privatekey for secret")
				c.SendLine(privateKey)
				c.ExpectString("filename of certchain for secret")
				c.SendLine(certChainFilename)
				c.ExpectEOF()
			}, func() {
				var meta core.Metadata
				var tlsSecret options.TlsSecret
				err := TlsSecretArgsInteractive(&meta, &tlsSecret)
				Expect(err).NotTo(HaveOccurred())
				Expect(tlsSecret.RootCaFilename).To(Equal(rootCa))
				Expect(tlsSecret.PrivateKeyFilename).To(Equal(privateKey))
				Expect(tlsSecret.CertChainFilename).To(Equal(certChainFilename))
			})
		})
	})
})
