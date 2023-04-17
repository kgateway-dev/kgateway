package secret

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options/contextoptions"

	. "github.com/onsi/ginkgo/v2"
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

	AfterEach(func() {
		helpers.UseDefaultClients()
	})

	expectMeta := func(meta core.Metadata) {
		Expect(meta.Namespace).To(Equal(secretNamespace))
		Expect(meta.Name).To(Equal(secretName))
	}

	Context("AWS", func() {
		It("should work", func() {
			var (
				accessKey    = "foo"
				secretKey    = "bar"
				sessionToken = "baz"
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
				c.ExpectString(awsPromptSessionToken)
				c.SendLine(sessionToken)
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
		// This test passes, but the key=value input is very fragile
		It("should work", func() {
			var (
				key1 = "Key1"
				val1 = "Val1"
				key2 = "Key2"
				val2 = "Val2"
			)
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(surveyutils.PromptInteractiveNamespace)
				c.SendLine(secretNamespace)
				c.ExpectString(surveyutils.PromptInteractiveResourceName)
				c.SendLine(secretName)

				c.ExpectString(azurePromptApiKeys)
				c.SendLine(fmt.Sprintf("%v=%v", key1, val1))

				c.ExpectString(azurePromptApiKeys)
				c.SendLine(fmt.Sprintf("%v=%v", key2, val2))

				c.ExpectString(azurePromptApiKeys)
				c.SendLine(`doesNotComeThrough=needsInvestigation`) // need to find a solution to the idiosyncrasy of slice input

				c.ExpectString(azurePromptApiKeys)
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				azureSecretOpts := options.Secret{
					AzureSecret: options.AzureSecret{
						ApiKeys: options.InputMapStringString{},
					},
				}
				opts, err := runCreateSecretCommand("azure", azureSecretOpts)
				Expect(err).NotTo(HaveOccurred())
				expectMeta(opts.Metadata)
				// This test passes, however, if the pseudoterminal used in testing behaved in the same way as the real
				// terminals used in testing it would fail.
				// In a real terminal, the "doesNotComeThrough": "needsInvestigation" key-value pair would be included.
				Expect(opts.Create.InputSecret.AzureSecret.ApiKeys.MustMap()).To(BeEquivalentTo(map[string]string{key1: val1, key2: val2}))
			})
		})
	})

	Context("Tls", func() {
		It("should work", func() {
			var (
				rootCa             = "foo"
				privateKey         = "bar"
				certChainFilename  = "baz"
				ocspStapleFilename = "qux"
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
				c.ExpectString(tlsPromptOcspStaple)
				c.SendLine(ocspStapleFilename)
				c.ExpectEOF()
			}, func() {
				tlsSecretOpts := options.Secret{
					TlsSecret: options.TlsSecret{
						RootCaFilename:     "",
						PrivateKeyFilename: "",
						CertChainFilename:  "",
						OCSPStapleFilename: "",
						Mock:               true,
					},
				}
				opts, err := runCreateSecretCommand("tls", tlsSecretOpts)
				Expect(err).NotTo(HaveOccurred())
				expectMeta(opts.Metadata)
				Expect(opts.Create.InputSecret.TlsSecret.RootCaFilename).To(Equal(rootCa))
				Expect(opts.Create.InputSecret.TlsSecret.PrivateKeyFilename).To(Equal(privateKey))
				Expect(opts.Create.InputSecret.TlsSecret.CertChainFilename).To(Equal(certChainFilename))
				Expect(opts.Create.InputSecret.TlsSecret.OCSPStapleFilename).To(Equal(ocspStapleFilename))
			})
		})
	})
})

func getMinCreateSecretOptions(secretOpts options.Secret) *options.Options {
	return &options.Options{
		Top: options.Top{
			Ctx: context.Background(),
			// These are all interactive tests
			ContextAccessible: contextoptions.ContextAccessible{
				Interactive: true,
			},
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
