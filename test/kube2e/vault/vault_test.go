package vault_test

import (
	"context"
	"fmt"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/test/gomega"
	"github.com/solo-io/gloo/test/services"
	skclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

// Needs AWS_SHARED_CREDENTIALS_FILE set or else causes issues re. AWS `NoCredentialProviders: no valid providers in chain.`.
var _ = Describe("Vault Tests", func() {
	const (
		vaultIrsaAwsRole  = "arn:aws:iam::802411188784:role/edge-e2e-test-irsa"
		iamServerIdHeader = "vault.gloo.example.com"
		vaultAwsRegion    = "us-east-2"
		vaultRole         = "edge-e2e-test-irsa"
		vaultSecretName   = "vaultsecret"
	)

	var (
		vaultInstance  *services.VaultInstance
		secretForVault *v1.Secret

		testNamespace string
		cfg           *rest.Config
		kubeClient    kubernetes.Interface
		kubeCoreCache corecache.KubeCoreCache
		secretClient  v1.SecretClient
		settings      *v1.Settings

		testCtx             context.Context
		testCancel          context.CancelFunc
		vaultSecretSettings *gloov1.Settings_VaultSecrets
		vaultClientInitMap  map[int]clients.VaultClientInitFunc
	)

	setVaultClientInitMap := func(idx int, vaultSettings *v1.Settings_VaultSecrets) {
		vaultClientInitMap[idx] = func() *vaultapi.Client {
			c, err := clients.VaultClientForSettings(vaultSettings)
			// Hitting an error here when we don't set the AccessToken & SecretToken in the aws settings...
			Expect(err).NotTo(HaveOccurred())
			return c
		}
	}

	// setupVaultSecret will
	// - initiate vault instance
	// - create a new secret
	// - wait up to 5 seconds to confirm the existence of the secret
	//
	// as-is, this function is not idempotent and should be run only once
	setupVaultSecret := func() {
		secretForVault = &v1.Secret{
			Kind: &v1.Secret_Tls{},
			Metadata: &core.Metadata{
				Name:      vaultSecretName,
				Namespace: testNamespace,
			},
		}

		vaultInstance.WriteSecret(secretForVault)
		Eventually(func(g Gomega) error {
			// https://developer.hashicorp.com/vault/docs/commands/kv/get
			s, err := vaultInstance.Exec("kv", "get", "-mount=secret", fmt.Sprintf("gloo/gloo.solo.io/v1/Secret/%s/%s", testNamespace, vaultSecretName))
			if err != nil {
				return err
			}
			g.Expect(s).NotTo(BeEmpty())
			return nil
		}, "5s", "500ms").ShouldNot(HaveOccurred())
	}

	BeforeEach(func() {
		testCtx, testCancel = context.WithCancel(ctx)
		//testNamespace = skhelpers.RandString(8)
		testNamespace = "gloo-system"
		vaultClientInitMap = make(map[int]clients.VaultClientInitFunc)

		// Set up Vault
		vaultInstance = vaultFactory.MustVaultInstance()
		err := vaultInstance.Run(testCtx)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		err = vaultInstance.EnableAWSSTSAuthMethod(vaultIrsaAwsRole, iamServerIdHeader, vaultAwsRegion)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		vaultSecretSettings = &gloov1.Settings_VaultSecrets{
			Address: vaultInstance.Address(),
			AuthMethod: &gloov1.Settings_VaultSecrets_Aws{
				Aws: &gloov1.Settings_VaultAwsAuth{
					IamServerIdHeader: iamServerIdHeader,
					Region:            vaultAwsRegion,
					VaultRole:         vaultRole,
					MountPath:         "aws",
				},
			},
		}

		// these are the settings that will be used by the secret client.
		//   SOME ISSUES:
		// 	   Empty AWS credentials (not having `AWS_SHARED_CREDENTIALS_FILE` envVar set) leads to `Error: NoCredentialProviders: no valid providers in chain. Deprecated`.
		//       Same thing happens in the non-kube test I was working on but I didn't catch it since that envVar is required for it in general... is this expected? or did they maybe have creds accidentally(?) set?
		//         If so, then what does this mean for the issue's scope / DoD? We'd maybe have to investigate what else would need to be updated to allow for it to work. Maybe just removing the login brought up below?
		//       Immediate login in `bootstrap/clients/vault.go:197` causes this.
		//         Could it be that we haven't done STS/IRSA stuff fast enough to get credentials before that?
		//         Maybe it's not configured correctly, which could be why we're not getting the credentials before login?
		//     NON-Empty credentials leads to: `unable to log in with AWS auth: Error making API request.\n\nURL: PUT http://127.0.0.1:8200/v1/auth/aws/login\nCode: 400. Errors:\n\n* entry for role gloo-edge-e2e-user not found`
		settings = &v1.Settings{
			WatchNamespaces: []string{testNamespace},
			SecretOptions: &v1.Settings_SecretOptions{
				Sources: []*v1.Settings_SecretOptions_Source{
					{
						Source: &v1.Settings_SecretOptions_Source_Vault{
							Vault: vaultSecretSettings,
						},
					},
					{
						Source: &v1.Settings_SecretOptions_Source_Kubernetes{},
					},
				},
			},
		}
	})

	AfterEach(func() {
		testCancel()
	})

	JustBeforeEach(func() {
		setupVaultSecret()
		setVaultClientInitMap(0, vaultSecretSettings)
		factory, err := clients.SecretFactoryForSettings(ctx,
			clients.SecretFactoryParams{
				Settings:           settings,
				SharedCache:        nil,
				Cfg:                &cfg,
				Clientset:          &kubeClient,
				VaultClientInitMap: vaultClientInitMap,
				KubeCoreCache:      &kubeCoreCache,
			})
		Expect(err).NotTo(HaveOccurred())
		secretClient, err = v1.NewSecretClient(ctx, factory)
		Expect(err).NotTo(HaveOccurred())
	})

	listSecret := func(g Gomega, secretName string) {
		l, err := secretClient.List(testNamespace, skclients.ListOpts{})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(l).NotTo(BeNil())
		kubeSecret, err := l.Find(testNamespace, secretName)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(kubeSecret).NotTo(BeNil())
	}

	When("using secretSource API", func() {
		When("using a vault secret source", func() {
			It("lists secrets", func() {
				//Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&vault.ResourceClient{}))
				Eventually(func(g Gomega) {
					listSecret(g, vaultSecretName)
				}, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Succeed())
			})
		})
	})
})
