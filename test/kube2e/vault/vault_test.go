package vault_test

import (
	"context"
	"fmt"
	vaultapi "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	. "github.com/solo-io/gloo/test/gomega"
	"github.com/solo-io/gloo/test/services"
	skclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/vault"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Needs AWS_SHARED_CREDENTIALS_FILE set or else causes issues re. AWS `NoCredentialProviders: no valid providers in chain.`.
var _ = Describe("Vault Tests", func() {
	const (
		vaultIrsaAwsRole  = "arn:aws:iam::802411188784:role/edge-e2e-test-irsa"
		iamServerIdHeader = "vault.gloo.example.com"
		vaultAwsRegion    = "us-east-2"
		vaultSecretName   = "vaultsecret"
		vaultSecretPath   = "dev"
	)

	var (
		vaultInstance *services.VaultInstance

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
	// - create a new secret
	// - wait up to 5 seconds to confirm the existence of the secret
	setupVaultSecret := func() {
		_, err := vaultInstance.Exec(
			"kv",
			"put",
			fmt.Sprintf("-mount=%s", vaultSecretPath),
			"gloo/gloo.solo.io/v1",
			"keys=test",
		)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) error {
			// https://developer.hashicorp.com/vault/docs/commands/kv/get
			s, err := vaultInstance.Exec(
				"kv",
				"get",
				fmt.Sprintf("-mount=%s", vaultSecretPath),
				"gloo/gloo.solo.io/v1",
			)
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

		err = vaultInstance.EnableAWSSTSAuthMethod(vaultIrsaAwsRole, iamServerIdHeader, vaultAwsRegion, vaultSecretPath)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		vaultSecretSettings = &gloov1.Settings_VaultSecrets{
			Address: vaultInstance.Address(),
			AuthMethod: &gloov1.Settings_VaultSecrets_Aws{
				Aws: &gloov1.Settings_VaultAwsAuth{
					IamServerIdHeader: iamServerIdHeader,
					Region:            vaultAwsRegion,
					MountPath:         "aws",
				},
			},
			PathPrefix: vaultSecretPath,
		}

		// these are the settings that will be used by the secret client.
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
				Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&vault.ResourceClient{}))
				Eventually(func(g Gomega) {
					listSecret(g, vaultSecretName)
				}, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Succeed())
			})
		})
	})
})
