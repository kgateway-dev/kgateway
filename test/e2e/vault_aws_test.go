package e2e_test

import (
	"github.com/aws/aws-sdk-go/aws/credentials"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	bootstrap "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/ginkgo/decorators"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	// These tests run using the following AWS ARN for the Vault Role
	// If you want to run these tests locally, ensure that your local AWS credentials match,
	// or use another role
	// https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-profiles.html
	vaultAwsRole   = "arn:aws:iam::802411188784:user/gloo-edge-e2e-user"
	vaultAwsRegion = "us-east-1"

	vaultRole = "vault-role"

	// The x-vault-awsiam-server-id is used to validate between Vault and AWS.
	// The value used in our Vault AWS auth settings and in Vault's client config must match.
	iamServerIdHeader = "vault.gloo.example.com"
)

var _ = Describe("Vault Secret Store (AWS Auth)", decorators.Vault, func() {

	var (
		testContext         *e2e.TestContextWithVault
		vaultSecretSettings *gloov1.Settings_VaultSecrets
		oauthSecret         *gloov1.Secret
		useCredentials      bool
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContextWithVault(testutils.AwsCredentials())
		testContext.BeforeEach()

		oauthSecret = &gloov1.Secret{
			Metadata: &core.Metadata{
				Name:      "oauth-secret",
				Namespace: writeNamespace,
			},
			Kind: &gloov1.Secret_Oauth{
				Oauth: &v1.OauthSecret{
					ClientSecret: "test",
				},
			},
		}

		testContext.ResourcesToCreate().Secrets = gloov1.SecretList{
			oauthSecret,
		}
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.SetRunSettings(&gloov1.Settings{
			SecretSource: &gloov1.Settings_VaultSecretSource{
				VaultSecretSource: vaultSecretSettings,
			},
			//SecretOptions: &gloov1.Settings_SecretOptions{
			//	Sources: []*gloov1.Settings_SecretOptions_Source{
			//		{
			//			Source: &gloov1.Settings_SecretOptions_Source_Vault{
			//				Vault: vaultSecretSettings,
			//			},
			//		},
			//		{
			//			Source: &gloov1.Settings_SecretOptions_Source_Kubernetes{},
			//		},
			//	},
			//},
		})
		testContext.RunVault()

		// We need to turn on Vault AWS Auth after it has started running
		var err error
		if useCredentials {
			err = testContext.VaultInstance().EnableAWSCredentialsAuthMethod(vaultSecretSettings, vaultAwsRole)
		} else {
			err = testContext.VaultInstance().EnableAWSSTSAuthMethod(vaultAwsRole, iamServerIdHeader, vaultAwsRegion)
		}
		Expect(err).NotTo(HaveOccurred())

		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("Vault Credentials", func() {
		BeforeEach(func() {
			useCredentials = true
			localAwsCredentials := credentials.NewSharedCredentials("", "")
			v, err := localAwsCredentials.Get()
			Expect(err).NotTo(HaveOccurred(), "can load AWS shared credentials")

			vaultSecretSettings = &gloov1.Settings_VaultSecrets{
				Address: testContext.VaultInstance().Address(),
				AuthMethod: &gloov1.Settings_VaultSecrets_Aws{
					Aws: &gloov1.Settings_VaultAwsAuth{
						VaultRole:       vaultRole,
						Region:          vaultAwsRegion,
						AccessKeyId:     v.AccessKeyID,
						SecretAccessKey: v.SecretAccessKey,
					},
				},
				PathPrefix: bootstrap.DefaultPathPrefix,
				RootKey:    bootstrap.DefaultRootKey,
			}
		})

		It("can read secret using resource client", func() {
			Eventually(func(g Gomega) {
				secret, err := testContext.TestClients().SecretClient.Read(
					oauthSecret.GetMetadata().GetNamespace(),
					oauthSecret.GetMetadata().GetName(),
					clients.ReadOpts{
						Ctx: testContext.Ctx(),
					})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secret.GetOauth().GetClientSecret()).To(Equal("test"))
			}, "5s", ".5s").Should(Succeed())
		})

		It("can pick up new secrets created by vault client ", func() {
			newSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "new-secret",
					Namespace: writeNamespace,
				},
				Kind: &gloov1.Secret_Oauth{
					Oauth: &v1.OauthSecret{
						ClientSecret: "new-secret",
					},
				},
			}

			err := testContext.VaultInstance().WriteSecret(newSecret)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				secret, err := testContext.TestClients().SecretClient.Read(
					newSecret.GetMetadata().GetNamespace(),
					newSecret.GetMetadata().GetName(),
					clients.ReadOpts{
						Ctx: testContext.Ctx(),
					})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secret.GetOauth().GetClientSecret()).To(Equal("new-secret"))
			}, "5s", ".5s").Should(Succeed())
		})

	})

	// TODO(fabiangonz98): Fix these tests
	Context("STS", func() {
		BeforeEach(func() {
			useCredentials = false
			vaultSecretSettings = &gloov1.Settings_VaultSecrets{
				Address: testContext.VaultInstance().Address(),
				AuthMethod: &gloov1.Settings_VaultSecrets_Aws{
					Aws: &gloov1.Settings_VaultAwsAuth{
						IamServerIdHeader: iamServerIdHeader,
						// TODO(fabiangonz98): Example didn't have this set
						VaultRole: vaultRole,
						Region:    vaultAwsRegion,
						MountPath: "aws",
					},
				},
				PathPrefix: bootstrap.DefaultPathPrefix,
				RootKey:    bootstrap.DefaultRootKey,
			}
		})

		// add the extra annotation
		JustBeforeEach(func() {
			// can't find helm settings on test start...
			// need to patch deployment(?) to add the annotation and restart?
			// gloo.gloo.serviceAccount.extraAnnotations: eks.amazonaws.com/role-arn: arn:aws:iam::802411188784:role/vault-role
		})

		It("can read secret using resource client", func() {
			Skip("Need to fix permissions and complete setup")
			Eventually(func(g Gomega) {
				secret, err := testContext.TestClients().SecretClient.Read(
					oauthSecret.GetMetadata().GetNamespace(),
					oauthSecret.GetMetadata().GetName(),
					clients.ReadOpts{
						Ctx: testContext.Ctx(),
					})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secret.GetOauth().GetClientSecret()).To(Equal("test"))
			}, "5s", ".5s").Should(Succeed())
		})

		It("can pick up new secrets created by vault client ", func() {
			Skip("Need to fix permissions and complete setup")
			newSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "new-secret",
					Namespace: writeNamespace,
				},
				Kind: &gloov1.Secret_Oauth{
					Oauth: &v1.OauthSecret{
						ClientSecret: "new-secret",
					},
				},
			}

			err := testContext.VaultInstance().WriteSecret(newSecret)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				secret, err := testContext.TestClients().SecretClient.Read(
					newSecret.GetMetadata().GetNamespace(),
					newSecret.GetMetadata().GetName(),
					clients.ReadOpts{
						Ctx: testContext.Ctx(),
					})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secret.GetOauth().GetClientSecret()).To(Equal("new-secret"))
			}, "5s", ".5s").Should(Succeed())
		})
	})

})
