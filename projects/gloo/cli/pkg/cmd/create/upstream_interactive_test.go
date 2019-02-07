package create_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	. "github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Upstream", func() {

	const (
		upstreamPrompt       = "What type of Upstream do you want to create?"
		awsRegionPrompt      = "What region are the AWS services in for this upstream?"
	    azureFunctionsPrompt = "What is the name of the Azure Functions app to associate with this upstream?"
	    awsSecretPrompt      = "Choose an AWS credentials secret to link to this upstream"
	    azureSecretPrompt    = "Choose an Azure credentials secret to link to this upstream"
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	It("kube doesn't support interactive", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.PressDown()
			c.PressDown()
			c.PressDown()
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(&upstream)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("interactive mode not currently available for type kube"))
		})
	})

	It("consul doesn't support interactive", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.PressDown()
			c.PressDown()
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(&upstream)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("interactive mode not currently available for type consul"))
		})
	})

	It("aws interactive errors with no secret", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.SendLine("")
			c.ExpectString(awsRegionPrompt)
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(&upstream)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("no AWS secrets found. create an AWS credentials secret using " +
				"glooctl create secret aws --help"))
		})
	})

	Context("aws interactive tests with secret", func() {

		const (
			awsSecretName = "aws-secret"
			awsSecretNamespace = "gloo-system"
			defaultAwsRegion = "us-east-1"
		)

		var (
			secretRef core.ResourceRef
		)

		BeforeEach(func() {
			secretClient := helpers.MustSecretClient()
			secret := &gloov1.Secret{
				Metadata: core.Metadata{
					Name:      awsSecretName,
					Namespace: awsSecretNamespace,
				},
				Kind: &gloov1.Secret_Aws{
					Aws: &gloov1.AwsSecret{
						SecretKey: "foo",
						AccessKey: "bar",
					},
				},
			}
			_, err := secretClient.Write(secret, clients.WriteOpts{})
			secretRef = core.ResourceRef{
				Name: awsSecretName,
				Namespace: awsSecretNamespace,
			}
			Expect(err).NotTo(HaveOccurred())
		})

		It("aws interactive with defaults", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(upstreamPrompt)
				c.SendLine("")
				c.ExpectString(awsRegionPrompt)
				c.SendLine("")
				c.ExpectString(awsSecretPrompt)
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				var upstream options.InputUpstream
				err := AddUpstreamFlagsInteractive(&upstream)
				Expect(err).NotTo(HaveOccurred())
				Expect(upstream.Aws.Secret).To(Equal(secretRef))
				Expect(upstream.Aws.Region).To(Equal(defaultAwsRegion))
			})
		})

		It("aws interactive with custom region", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(upstreamPrompt)
				c.SendLine("")
				c.ExpectString(awsRegionPrompt)
				c.SendLine("custom-region")
				c.ExpectString(awsSecretPrompt)
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				var upstream options.InputUpstream
				err := AddUpstreamFlagsInteractive(&upstream)
				Expect(err).NotTo(HaveOccurred())
				Expect(upstream.Aws.Secret).To(Equal(secretRef))
				Expect(upstream.Aws.Region).To(Equal("custom-region"))
			})
		})
	})

	It("static upstream no hosts", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.PressDown()
			c.PressDown()
			c.PressDown()
			c.PressDown()
			c.SendLine("")
			c.ExpectString("Add another host for this upstream (empty to skip)? []")
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(&upstream)
			Expect(err).NotTo(HaveOccurred())
			Expect(upstream.Static.Hosts).To(BeNil())
		})
	})

	PIt("static upstream hosts", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.PressDown()
			c.PressDown()
			c.PressDown()
			c.PressDown()
			c.SendLine("")
			c.ExpectString("Add another host for this upstream (empty to skip)? []")
			c.SendLine("foo")
			c.SendLine("") // can not figure out how to advance in this case, some idiosyncrasy with the slice prompt
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(&upstream)
			Expect(err).NotTo(HaveOccurred())
			Expect(upstream.Static.Hosts).To(BeEquivalentTo([]string{"foo", "bar"}))
		})
	})

	It("azure interactive errors with no secret", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.PressDown()
			c.SendLine("")
			c.ExpectString(azureFunctionsPrompt)
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(&upstream)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("no Azure secrets found. create an Azure credentials secret using " +
				"glooctl create secret azure --help"))
		})
	})

	Context("azure interactive tests with secret", func() {

		const (
			azureSecretName      = "azure-secret"
			azureSecretNamespace = "gloo-system"
		)

		var (
			secretRef core.ResourceRef
		)

		BeforeEach(func() {
			secretClient := helpers.MustSecretClient()
			secret := &gloov1.Secret{
				Metadata: core.Metadata{
					Name:      azureSecretName,
					Namespace: azureSecretNamespace,
				},
				Kind: &gloov1.Secret_Azure{
					Azure: &gloov1.AzureSecret{
						ApiKeys: map[string]string{
							"foo": "bar",
						},
					},
				},
			}
			_, err := secretClient.Write(secret, clients.WriteOpts{})
			secretRef = core.ResourceRef{
				Name: azureSecretName,
				Namespace: azureSecretNamespace,
			}
			Expect(err).NotTo(HaveOccurred())
		})

		It("azure interactive with defaults", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(upstreamPrompt)
				c.PressDown()
				c.SendLine("")
				c.ExpectString(azureFunctionsPrompt)
				c.SendLine("")
				c.ExpectString(azureSecretPrompt)
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				var upstream options.InputUpstream
				err := AddUpstreamFlagsInteractive(&upstream)
				Expect(err).NotTo(HaveOccurred())
				Expect(upstream.Azure.Secret).To(Equal(secretRef))
				Expect(upstream.Azure.FunctionAppName).To(Equal(""))
			})
		})

		It("azure interactive with custom region", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(upstreamPrompt)
				c.PressDown()
				c.SendLine("")
				c.ExpectString(azureFunctionsPrompt)
				c.SendLine("custom")
				c.ExpectString(azureSecretPrompt)
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				var upstream options.InputUpstream
				err := AddUpstreamFlagsInteractive(&upstream)
				Expect(err).NotTo(HaveOccurred())
				Expect(upstream.Azure.Secret).To(Equal(secretRef))
				Expect(upstream.Azure.FunctionAppName).To(Equal("custom"))
			})
		})
	})

})

