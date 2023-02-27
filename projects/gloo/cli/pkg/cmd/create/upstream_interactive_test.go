package create_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	. "github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Upstream Interactive Mode", func() {

	const (
		upstreamPrompt       = "What type of Upstream do you want to create?"
		awsRegionPrompt      = "What region are the AWS services in for this upstream?"
		azureFunctionsPrompt = "What is the name of the Azure Functions app to associate with this upstream?"
		awsSecretPrompt      = "Choose an AWS credentials secret to link to this upstream"
		azureSecretPrompt    = "Choose an Azure credentials secret to link to this upstream"
	)

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() { cancel() })

	It("should not be allowed for Kube", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.SendLine("kube")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(ctx, &upstream)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("interactive mode not currently available for type kube"))
		})
	})

	It("should not be allowed for Consul", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.SendLine("consul")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(ctx, &upstream)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("interactive mode not currently available for type consul"))
		})
	})

	It("should error out for AWS when there's no secret", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.SendLine("aws")
			c.ExpectString(awsRegionPrompt)
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(ctx, &upstream)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("no AWS secrets found. create an AWS credentials secret using " +
				"glooctl create secret aws --help"))
		})
	})

	Context("AWS with secret", func() {

		const (
			awsSecretName      = "aws-secret"
			awsSecretNamespace = "gloo-system"
			defaultAwsRegion   = "us-east-1"
		)

		var (
			secretRef *core.ResourceRef
		)

		BeforeEach(func() {
			secretClient := helpers.MustSecretClient(ctx)
			secret := &gloov1.Secret{
				Metadata: &core.Metadata{
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
			secretRef = &core.ResourceRef{
				Name:      awsSecretName,
				Namespace: awsSecretNamespace,
			}
			Expect(err).NotTo(HaveOccurred())
		})

		It("should work with defaults", func() {
			localSecretRef := secretRef
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(upstreamPrompt)
				c.SendLine("aws")
				c.ExpectString(awsRegionPrompt)
				c.SendLine("")
				c.ExpectString(awsSecretPrompt)
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				var upstream options.InputUpstream
				err := AddUpstreamFlagsInteractive(ctx, &upstream)
				Expect(err).NotTo(HaveOccurred())
				Expect(&upstream.Aws.Secret).To(matchers.MatchProto(localSecretRef))
				Expect(upstream.Aws.Region).To(Equal(defaultAwsRegion))
			})
		})

		It("should work with custom region", func() {
			localSecretRef := secretRef
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(upstreamPrompt)
				c.SendLine("aws")
				c.ExpectString(awsRegionPrompt)
				c.SendLine("custom-region")
				c.ExpectString(awsSecretPrompt)
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				var upstream options.InputUpstream
				err := AddUpstreamFlagsInteractive(ctx, &upstream)
				Expect(err).NotTo(HaveOccurred())
				Expect(&upstream.Aws.Secret).To(matchers.MatchProto(localSecretRef))
				Expect(upstream.Aws.Region).To(Equal("custom-region"))
			})
		})
	})

	It("should work for static with no hosts", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.SendLine("static")
			c.ExpectString("Add another host for this upstream (empty to skip)? []")
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(ctx, &upstream)
			Expect(err).NotTo(HaveOccurred())
			Expect(upstream.Static.Hosts).To(BeNil())
		})
	})

	It("should work for static with hosts", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.SendLine("static")
			c.ExpectString("Add another host for this upstream (empty to skip)? []")
			c.SendLine("foo")
			c.ExpectString("Add another host for this upstream (empty to skip)? [foo]")
			c.SendLine("bar")
			c.ExpectString("Add another host for this upstream (empty to skip)? [foo bar]")
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(ctx, &upstream)
			Expect(err).NotTo(HaveOccurred())
			Expect(upstream.Static.Hosts).To(BeEquivalentTo([]string{"foo", "bar"}))
		})
	})

	It("should error out for Azure when there's no secret", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString(upstreamPrompt)
			c.SendLine("azure")
			c.ExpectString(azureFunctionsPrompt)
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var upstream options.InputUpstream
			err := AddUpstreamFlagsInteractive(ctx, &upstream)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("no Azure secrets found. create an Azure credentials secret using " +
				"glooctl create secret azure --help"))
		})
	})

	Context("Azure with secret", func() {

		const (
			azureSecretName      = "azure-secret"
			azureSecretNamespace = "gloo-system"
		)

		var (
			secretRef core.ResourceRef
		)

		BeforeEach(func() {
			secretClient := helpers.MustSecretClient(ctx)
			secret := &gloov1.Secret{
				Metadata: &core.Metadata{
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
				Name:      azureSecretName,
				Namespace: azureSecretNamespace,
			}
			Expect(err).NotTo(HaveOccurred())
		})

		It("should work with default function app name", func() {
			// copy the secret ref ttto appease the race detector
			localSecretRef := secretRef
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(upstreamPrompt)
				c.SendLine("azure")
				c.ExpectString(azureFunctionsPrompt)
				c.SendLine("")
				c.ExpectString(azureSecretPrompt)
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				var upstream options.InputUpstream
				err := AddUpstreamFlagsInteractive(ctx, &upstream)
				Expect(err).NotTo(HaveOccurred())
				Expect(upstream.Azure.Secret).To(Equal(localSecretRef))
				Expect(upstream.Azure.FunctionAppName).To(Equal(""))
			})
		})

		It("should work with custom function app name", func() {
			localSecretRef := secretRef

			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString(upstreamPrompt)
				c.SendLine("azure")
				c.ExpectString(azureFunctionsPrompt)
				c.SendLine("custom")
				c.ExpectString(azureSecretPrompt)
				c.SendLine("")
				c.ExpectEOF()
			}, func() {
				var upstream options.InputUpstream
				err := AddUpstreamFlagsInteractive(ctx, &upstream)
				Expect(err).NotTo(HaveOccurred())
				Expect(upstream.Azure.Secret).To(Equal(localSecretRef))
				Expect(upstream.Azure.FunctionAppName).To(Equal("custom"))
			})
		})
	})

})
