package vault_test

import (
	"context"
	"fmt"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
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

// needs AWS_SHARED_CREDENTIALS_FILE set or else causes issues re. AWS "NoCredentialProviders".
var _ = Describe("Vault Tests", func() {
	const (
		vaultAwsRole      = "arn:aws:iam::802411188784:user/gloo-edge-e2e-user"
		iamServerIdHeader = "vault.gloo.example.com"
		vaultAwsRegion    = "us-east-1"
		vaultRole         = "vault-role"
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
		testNamespace = skhelpers.RandString(8)
		vaultClientInitMap = make(map[int]clients.VaultClientInitFunc)

		// Set up Vault
		vaultInstance = vaultFactory.MustVaultInstance()
		err := vaultInstance.Run(testCtx)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		//err = vaultInstance.EnableAWSCredentialsAuthMethod(vaultSecretSettings, vaultAwsRole)
		err = vaultInstance.EnableAWSSTSAuthMethod(vaultAwsRole, iamServerIdHeader, vaultAwsRegion)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		//localAwsCredentials := credentials.NewSharedCredentials("", "aws_e2e")
		//v, err := localAwsCredentials.Get()
		//Expect(err).NotTo(HaveOccurred(), "can load AWS shared credentials")
		vaultSecretSettings = &gloov1.Settings_VaultSecrets{
			Address: vaultInstance.Address(),
			AuthMethod: &gloov1.Settings_VaultSecrets_Aws{
				Aws: &gloov1.Settings_VaultAwsAuth{
					IamServerIdHeader: iamServerIdHeader,
					Region:            vaultAwsRegion,
					MountPath:         "aws",
				},
			},
			PathPrefix: bootstrap.DefaultPathPrefix,
			RootKey:    bootstrap.DefaultRootKey,
		}

		// these are the settings that will be used by the secret client.
		// - can not use vaultClientInitMap + extra client like done in bootstrap_clients_test
		// 		Empty AWS credentials lead to `Error: NoCredentialProviders: no valid providers in chain. Deprecated`.
		//      Immediate login in `bootstrap/clients/vault.go:197` causes this. Could it be that we haven't done STS/IRSA stuff fast enough to get credentials before that?
		//        The only other thing I can think of is that in the non-kube2e setup where this DID NOT happen we first set up Vault with its aws role setup, THEN Gloo.
		//        But in Kube2e tests we setup Gloo BEFORE Vault, so maybe we're not getting credentials in time?
		// 		afaik, when field tested without credentials being required this was not an issue, so it's just due to the initMap?
		//      I also didn't hit this issue in the non kube2e setup (vault_aws_test.go) and got an error regarding a bad role, so seems exclusive to how stuff is set up here...
		// - can not set during the gloo installation because, at least with this set up, it is installed before the vault instance is running and we won't have an address
		// - how can i set the secret run settings AFTER gloo installed?
		//var settings *v1.Settings
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

		// install gloo after
	})

	AfterEach(func() {
		testCancel()
	})

	JustBeforeEach(func() {
		setupVaultSecret()
		setVaultClientInitMap(0, vaultSecretSettings)
		//// having empty aws credentials in the vaultClientInitMap cause this to fail, since vault tries to log in (immediately?) and fails due to missing credentials.
		//// we need valid credentials during the run for it to continue, but that defeats the purpose of STS/IRSA...
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
		//l, err := resourceClientSet.SecretClient().List(testNamespace, skclients.ListOpts{})
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
