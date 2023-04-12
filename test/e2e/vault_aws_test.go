package e2e_test

import (
	"github.com/aws/aws-sdk-go/aws/credentials"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

const (
	// These tests run using the following AWS ARN for the Vault Role
	// If you want to run these tests locally, ensure that your local AWS credentials match,
	// or use another role
	// https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-profiles.html
	vaultAwsRole   = "arn:aws:iam::802411188784:user/gloo-edge-e2e-user"
	vaultAwsRegion = "us-east-1"

	vaultRole = "vault-role"
)

var _ = Describe("Vault Secret Store (AWS Auth)", func() {

	var (
		testContext *e2e.TestContextWithVault
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContextWithVault(testutils.AwsCredentials())
		testContext.BeforeEach()

		localAwsCredentials := credentials.NewSharedCredentials("", "")
		v, err := localAwsCredentials.Get()
		Expect(err).NotTo(HaveOccurred(), "can load AWS shared credentials")

		vaultSecretSettings := &gloov1.Settings_VaultSecrets{
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

		testContext.SetRunSettings(&gloov1.Settings{
			SecretSource: &gloov1.Settings_VaultSecretSource{
				VaultSecretSource: vaultSecretSettings,
			},
		})

		testContext.RunVault()

		// We need to turn on Vault AWS Auth after it has started running
		err = testContext.VaultInstance().EnableAWSAuthMethod(vaultSecretSettings, vaultAwsRole)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("Oauth Secret", func() {

		var (
			oauthSecret *gloov1.Secret
		)

		BeforeEach(func() {
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

})
