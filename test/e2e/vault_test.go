package e2e_test

import (
	"bytes"
	"context"
	"net/http"
	"net/url"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/aws/aws-sdk-go/aws/credentials"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	vaultapi "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Vault e2e", func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		vaultInstance *services.VaultInstance
		err           error
	)

	BeforeEach(func() {
		testutils.ValidateRequirementsAndNotifyGinkgo(
			testutils.Vault(),
			testutils.LinuxOnly("docker image we get the executable from is only built for linux"),
			testutils.AwsCredentials(),
		)

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		if vaultInstance != nil {
			err = vaultInstance.Clean()
			Expect(err).NotTo(HaveOccurred())
		}
		cancel()
	})

	Context("with vault", func() {
		var (
			vaultClient        *vaultapi.Client
			vaultResources     factory.ResourceClientFactory
			vaultFactoryConfig *services.VaultFactoryConfig
			secretClient       gloov1.SecretClient
			settings           *gloov1.Settings_VaultSecrets
		)
		startVault := func() {
			vaultFactory, err := services.NewVaultFactoryForConfig(vaultFactoryConfig)
			Expect(err).NotTo(HaveOccurred())
			vaultInstance, err = vaultFactory.NewVaultInstance()
			Expect(err).NotTo(HaveOccurred())
			err = vaultInstance.Run()
			Expect(err).NotTo(HaveOccurred())

		}
		eventuallyReadsTestSecret := func() {
			Eventually(func(g Gomega) {
				sec, err := secretClient.Read("gloo-system", "test-secret", clients.ReadOpts{}.WithDefaults())
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sec.String()).NotTo(BeEmpty())
			}, "5s", ".5s").Should(Succeed())
		}

		secretForWriteTests := &gloov1.Secret{
			Kind: &gloov1.Secret_Oauth{Oauth: &v1.OauthSecret{
				ClientSecret: "test",
			}},
			Metadata: &core.Metadata{
				Name:      "test-write-secret",
				Namespace: "gloo-system",
			},
		}

		JustBeforeEach(func() {
			// Write a known-good test secret to attempt to read
			err = writeTestSecret()

			// Set up our vault client
			vaultClient, err = bootstrap.VaultClientForSettings(settings)
			Expect(err).NotTo(HaveOccurred())
			vaultResources = bootstrap.NewVaultSecretClientFactory(vaultClient, "secret", bootstrap.DefaultRootKey)
			secretClient, err = gloov1.NewSecretClient(ctx, vaultResources)
		})

		Context("token auth", func() {
			BeforeEach(func() {
				vaultFactoryConfig = &services.VaultFactoryConfig{}
				startVault()
				settings = &gloov1.Settings_VaultSecrets{
					Address: vaultInstance.Address(),
					AuthMethod: &gloov1.Settings_VaultSecrets_AccessToken{
						AccessToken: services.DefaultVaultToken,
					},
				}
			})
			It("reads secret", func() {
				eventuallyReadsTestSecret()
			})
			It("writes secret", func() {
				_, err = secretClient.Write(secretForWriteTests, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())
				sec, err := vaultInstance.Exec("kv", "get", "-mount=secret", "gloo/gloo.solo.io/v1/Secret/gloo-system/test-write-secret")
				Expect(err).NotTo(HaveOccurred())
				Expect(sec).NotTo(BeEmpty())
			})
		})
		Context("aws auth", func() {
			BeforeEach(func() {
				Skip("until AWS creds are in cloudbuild")

				vaultFactoryConfig = &services.VaultFactoryConfig{}
				startVault()
				localAwsCredentials := credentials.NewSharedCredentials("", "")
				v, err := localAwsCredentials.Get()
				if err != nil {
					Fail("no AWS creds available")
				}

				settings = &gloov1.Settings_VaultSecrets{
					Address: vaultInstance.Address(),
					AuthMethod: &gloov1.Settings_VaultSecrets_Aws{
						Aws: &gloov1.Settings_VaultAwsAuth{
							VaultRole:       "vault-role",
							Region:          "us-east-1",
							AccessKeyId:     v.AccessKeyID,
							SecretAccessKey: v.SecretAccessKey,
						},
					},
				}
				err = vaultInstance.EnableAWSAuthMethod(settings)
				Expect(err).NotTo(HaveOccurred())
			})
			It("reads secret", func() {
				eventuallyReadsTestSecret()
			})
			It("writes secret", func() {
				_, err = secretClient.Write(secretForWriteTests, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())
				sec, err := vaultInstance.Exec("kv", "get", "-mount=secret", "gloo/gloo.solo.io/v1/Secret/gloo-system/test-write-secret")
				Expect(err).NotTo(HaveOccurred())
				Expect(sec).NotTo(BeEmpty())

			})
		})
	})
})

// write a simple test secret to a known-good path to check that we can read it
func writeTestSecret() error {
	body := bytes.NewReader([]byte(`{"data":{"metadata":{"name":"test-secret", "namespace":"gloo-system"},"oauth":{"clientSecret":"foo"}}}`))
	u := &url.URL{
		Scheme: "http",
		Host:   "localhost:8200",
		Path:   "/v1/secret/data/gloo/gloo.solo.io/v1/Secret/gloo-system/test-secret",
	}

	req, err := http.NewRequest(http.MethodPost, u.String(), body)
	if err != nil {
		return err
	}

	req.Header.Add("X-Vault-Token", services.DefaultVaultToken)
	_, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return nil
}
