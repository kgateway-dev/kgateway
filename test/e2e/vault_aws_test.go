package e2e_test

import (
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	bootstrap "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/ginkgo/decorators"
	"github.com/solo-io/gloo/test/gomega/assertions"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.opencensus.io/stats/view"
)

const (
	// These tests run using the following AWS ARN for the Vault Role
	// If you want to run these tests locally, ensure that your local AWS credentials match,
	// or use another role
	// https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-profiles.html
	// Please note that although this is used as a "role" in vault (the value is written to "auth/aws/role/vault-role")
	// it is actually an aws user so if running locally *user* and not the role that gets created during manual setup
	vaultAwsRole   = "arn:aws:iam::802411188784:user/gloo-edge-e2e-user"
	vaultAwsRegion = "us-east-1"

	vaultRole = "vault-role"
)

var _ = Describe("Vault Secret Store (AWS Auth)", decorators.Vault, func() {

	var (
		testContext         *e2e.TestContextWithVault
		vaultSecretSettings *gloov1.Settings_VaultSecrets
		oauthSecret         *gloov1.Secret
	)

	BeforeEach(func() {
		resetViews()
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
		})
		testContext.RunVault()

		// We need to turn on Vault AWS Auth after it has started running
		err := testContext.VaultInstance().EnableAWSCredentialsAuthMethod(vaultSecretSettings, vaultAwsRole, []string{"default_ttl=10s", "max_ttl=10s"})
		Expect(err).NotTo(HaveOccurred())

		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("Vault Credentials", func() {
		BeforeEach(func() {
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
						LeaseIncrement:  5,
					},
				},
				PathPrefix: bootstrap.DefaultPathPrefix,
				RootKey:    bootstrap.DefaultRootKey,
			}
		})

		It("can read secret using resource client throughout the token lifecycle", func() {

			var (
				secret *gloov1.Secret
				err    error
			)

			getSecret := func(g Gomega) {
				secret, err = testContext.TestClients().SecretClient.Read(
					oauthSecret.GetMetadata().GetNamespace(),
					oauthSecret.GetMetadata().GetName(),
					clients.ReadOpts{
						Ctx: testContext.Ctx(),
					})
				g.ExpectWithOffset(1, err).NotTo(HaveOccurred())
				g.ExpectWithOffset(1, secret.GetOauth().GetClientSecret()).To(Equal("test"))
			}

			// TEST CASE: We can read with a token
			Eventually(getSecret, "5s", ".5s").Should(Succeed())

			// Check the metrics - we should have one login success and one renewal beacuse the LifetimeWatcher renews as soon as it is started
			assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, BeZero())
			assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(vault.MLoginFailures, BeZero())
			assertions.ExpectStatSumMatches(vault.MLoginSuccesses, Equal(1))
			assertions.ExpectStatLastValueMatches(vault.MLastRenewFailure, BeZero())
			assertions.ExpectStatLastValueMatches(vault.MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(vault.MRenewFailures, BeZero())
			assertions.ExpectStatSumMatches(vault.MRenewSuccesses, Equal(1))

			// TEST CASE: We can read the secret with a renewed token
			// We have used up (0-5] seconds of the 10 second lease, if we sleep for 5 more seconds, we should
			// have to renew the lease again without needed to re-login
			time.Sleep(5 * time.Second)

			// Check the metrics - we should have an additional renewal
			assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, BeZero())
			assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(vault.MLoginFailures, BeZero())
			assertions.ExpectStatSumMatches(vault.MLoginSuccesses, Equal(1))
			assertions.ExpectStatLastValueMatches(vault.MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(vault.MRenewFailures, BeZero())
			assertions.ExpectStatSumMatches(vault.MRenewSuccesses, Equal(2))

			// Don't need the "Eventually" here, because everything is already up and running
			getSecret(Default)

			// TEST CASE: we can read the secret after the token expires and login is re-run
			// We have used up (5-10] seconds of the 10 second lease, if we sleep for 7 seconds, we should see a re-login
			// Sleep a little extra to give login time to complete
			time.Sleep(7 * time.Second)

			// Check the metrics - we should have an additional renewal failure, and an additional login success
			// plus 2-3 more renewal successes. The uncertainty is due jitter used caculate the time to renew:
			// "For a given lease duration, we want to allow 80-90% of that to elapse,"
			// This means that even though we have a lease interval of 4 seconds, the renewal will happen sooner
			// This leaves room for the possibility of a 3rd renewal happening before the lease expires
			assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, BeZero())
			assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(vault.MLoginFailures, BeZero())

			assertions.ExpectStatLastValueMatches(vault.MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(vault.MRenewFailures, Equal(1))
			assertions.ExpectStatSumMatches(vault.MRenewSuccesses, BeNumerically("~", 4, 5))
			assertions.ExpectStatSumMatches(vault.MLoginSuccesses, Equal(2))

			getSecret(Default)
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

func resetViews() {
	views := []*view.View{
		vault.MLastLoginSuccessView,
		vault.MLoginFailuresView,
		vault.MLoginSuccessesView,
		vault.MLastLoginFailureView,
		vault.MLastRenewFailureView,
		vault.MLastRenewSuccessView,
		vault.MRenewFailuresView,
		vault.MRenewSuccessesView,
	}
	view.Unregister(views...)
	_ = view.Register(views...)
	assertions.ExpectStatLastValueMatches(vault.MLastLoginSuccess, BeZero())
	assertions.ExpectStatLastValueMatches(vault.MLastLoginFailure, BeZero())
	assertions.ExpectStatSumMatches(vault.MLoginSuccesses, BeZero())
	assertions.ExpectStatSumMatches(vault.MLoginFailures, BeZero())
	assertions.ExpectStatLastValueMatches(vault.MLastRenewFailure, BeZero())
	assertions.ExpectStatLastValueMatches(vault.MLastRenewSuccess, BeZero())
	assertions.ExpectStatSumMatches(vault.MRenewFailures, BeZero())
	assertions.ExpectStatSumMatches(vault.MRenewSuccesses, BeZero())
}
