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
			var (
				accessKey = "foo"
				secretKey = "foo"
			)
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(surveyutils.PromptInteractiveNamespace)
				c.SendLine(secretNamespace)
				c.ExpectString(surveyutils.PromptInteractiveResourceName)
				c.SendLine(secretName)
				c.ExpectString(awsPromptAccessKey)
				c.SendLine(accessKey)
				c.ExpectString(awsPromptSecretKey)
				c.SendLine(secretKey)
				c.ExpectEOF()
			}, func() {
				awsSecretOpts := options.Secret{
					AwsSecret: options.AwsSecret{
						AccessKey: flagDefaultAwsAccessKey,
						SecretKey: flagDefaultAwsSecretKey,
					},
				}
				opts, err := runCreateSecretCommand("aws", awsSecretOpts)
				Expect(err).NotTo(HaveOccurred())
				expectMeta(opts.Metadata)
				Expect(opts.Create.InputSecret.AwsSecret.AccessKey).To(Equal(accessKey))
				Expect(opts.Create.InputSecret.AwsSecret.SecretKey).To(Equal(secretKey))
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
				c.ExpectString(surveyutils.PromptInteractiveNamespace)
				c.SendLine(secretNamespace)
				c.ExpectString(surveyutils.PromptInteractiveResourceName)
				c.SendLine(secretName)
				c.ExpectString(tlsPromptRootCa)
				c.SendLine(rootCa)
				c.ExpectString(tlsPromptPrivateKey)
				c.SendLine(privateKey)
				c.ExpectString(tlsPromptCertChain)
				c.SendLine(certChainFilename)
				c.ExpectEOF()
			}, func() {
				tlsSecretOpts := options.Secret{
					TlsSecret: options.TlsSecret{
						RootCaFilename:     "",
						PrivateKeyFilename: "",
						CertChainFilename:  "",
						Mock:               true,
					},
				}
				opts, err := runCreateSecretCommand("tls", tlsSecretOpts)
				Expect(err).NotTo(HaveOccurred())
				expectMeta(opts.Metadata)
				Expect(opts.Create.InputSecret.TlsSecret.RootCaFilename).To(Equal(rootCa))
				Expect(opts.Create.InputSecret.TlsSecret.PrivateKeyFilename).To(Equal(privateKey))
				Expect(opts.Create.InputSecret.TlsSecret.CertChainFilename).To(Equal(certChainFilename))
			})
		})
	})
})

func getMinCreateSecretOptions(secretOpts options.Secret) *options.Options {
	return &options.Options{
		Top: options.Top{
			Ctx: context.Background(),
			// These are all interactive tests
			Interactive: true,
		},
		Metadata: core.Metadata{},
		Create: options.Create{
			InputSecret: secretOpts,
			// Do not create the resources during the tests
			DryRun: true,
		},
	}
}

func runCreateSecretCommand(secretType string, secretOpts options.Secret) (*options.Options, error) {
	opts := getMinCreateSecretOptions(secretOpts)
	cmd := CreateCmd(opts)
	cmd.SetArgs([]string{secretType})
	return opts, cmd.Execute()
}
