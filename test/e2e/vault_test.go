package e2e_test

import (
    "bytes"
    "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
    "github.com/solo-io/gloo/test/e2e"
    "net/http"
    "net/url"

    "github.com/solo-io/gloo/test/testutils"

    gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
    v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
    "github.com/solo-io/gloo/test/services"
    "github.com/solo-io/solo-kit/pkg/api/v1/clients"
    "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = FDescribe("Vault Secret Store (Token Auth)", func() {

    var (
        testContext *e2e.TestContext
    )

    BeforeEach(func() {
        e2eFactoryWithVault := &e2e.TestContextFactory{
            EnvoyFactory: envoyFactory,
            VaultFactory: vaultFactory,
        }
        testContext = e2eFactoryWithVault.NewTestContext(testutils.Vault())
        testContext.BeforeEach()

        testContext.SetRunSettings(&gloov1.Settings{
            SecretSource: &gloov1.Settings_VaultSecretSource{
                VaultSecretSource: &gloov1.Settings_VaultSecrets{
                    Address: testContext.VaultInstance().Address(),
                    AuthMethod: &gloov1.Settings_VaultSecrets_AccessToken{
                        AccessToken: services.DefaultVaultToken,
                    },
                    PathPrefix: bootstrap.DefaultPathPrefix,
                    RootKey:    bootstrap.DefaultRootKey,
                },
            },
        })

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

        FIt("can read secret using resource client", func() {
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
