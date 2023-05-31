package gloo_test

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/vault"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/k8s-utils/kubeutils"
	skclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Kubernetes tests for clients generated from projects/gloo/pkg/bootstrap/clients
var _ = Describe("Bootstrap Clients", func() {

	Context("Kube Client Factory", func() {

		It("should set kube rate limits", func() {
			var cfg *rest.Config
			settings := &v1.Settings{
				ConfigSource: &v1.Settings_KubernetesConfigSource{
					KubernetesConfigSource: &v1.Settings_KubernetesCrds{},
				},
				Kubernetes: &v1.Settings_KubernetesConfiguration{
					RateLimits: &v1.Settings_KubernetesConfiguration_RateLimits{
						QPS:   100.5,
						Burst: 1000,
					},
				},
			}
			params := clients.NewConfigFactoryParams(
				settings,
				nil,
				nil,
				&cfg,
				nil,
			)

			kubefactory, err := clients.ConfigFactoryForSettings(params, v1.UpstreamCrd)

			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).ToNot(BeNil())
			Expect(kubefactory.(*factory.KubeResourceClientFactory).Cfg).To(Equal(cfg))

			Expect(cfg.QPS).To(Equal(float32(100.5)))
			Expect(cfg.Burst).To(Equal(1000))
		})

	})

	Context("Artifact Client", func() {

		var (
			testNamespace string

			cfg           *rest.Config
			kubeClient    kubernetes.Interface
			kubeCoreCache corecache.KubeCoreCache
		)

		BeforeEach(func() {
			var err error

			cfg, err = kubeutils.GetConfig("", "")
			Expect(err).NotTo(HaveOccurred())
			kubeClient = resourceClientset.KubeClients()

			testNamespace = skhelpers.RandString(8)
			_, err = kubeClient.CoreV1().Namespaces().Create(ctx, &kubev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespace,
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			kubeCoreCache, err = corecache.NewKubeCoreCacheWithOptions(ctx, kubeClient, time.Hour, []string{testNamespace})
			Expect(err).NotTo(HaveOccurred())

		})

		AfterEach(func() {
			err := kubeClient.CoreV1().Namespaces().Delete(ctx, testNamespace, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("artifacts as config maps", func() {

			var (
				artifactClient v1.ArtifactClient
			)

			BeforeEach(func() {
				_, err := kubeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx,
					&kubev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "cfg",
							Namespace: testNamespace,
						},
						Data: map[string]string{
							"test": "data",
						},
					}, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				settings := &v1.Settings{
					ArtifactSource:  &v1.Settings_KubernetesArtifactSource{},
					WatchNamespaces: []string{testNamespace},
				}

				factory, err := clients.ArtifactFactoryForSettings(ctx,
					settings,
					nil,
					&cfg,
					&kubeClient,
					&kubeCoreCache,
					&api.Client{},
					"artifacts")
				Expect(err).NotTo(HaveOccurred())
				artifactClient, err = v1.NewArtifactClient(ctx, factory)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should work with artifacts", func() {
				artifact, err := artifactClient.Read(testNamespace, "cfg", skclients.ReadOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
				Expect(artifact.GetMetadata().Name).To(Equal("cfg"))
				Expect(artifact.Data["test"]).To(Equal("data"))
			})
		})
	})

	Context("Secret Client", func() {
		const (
			kubeSecretName  = "kubesecret"
			vaultSecretName = "vaultsecret"
		)
		var (
			vaultInstance  *services.VaultInstance
			secretForVault *v1.Secret

			testNamespace      string
			cfg                *rest.Config
			kubeClient         kubernetes.Interface
			kubeCoreCache      corecache.KubeCoreCache
			secretClient       v1.SecretClient
			settings           *v1.Settings
			vaultClientInitMap map[int]clients.VaultClientInitFunc

			testCtx    context.Context
			testCancel context.CancelFunc
		)

		// setupKubeSecret will
		// - initiate kube clients
		// - create a namespace
		// - create a kubeCoreCache
		// - create a new secret
		// - wait up to 5 seconds to confirm the existence of the secret
		//
		// as-is, this function is not idempotent and should be run only once
		setupKubeSecret := func() {
			var err error
			cfg, err = kubeutils.GetConfig("", "")
			Expect(err).NotTo(HaveOccurred())
			kubeClient = resourceClientset.KubeClients()

			_, err = kubeClient.CoreV1().Namespaces().Create(ctx, &kubev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespace,
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			kubeCoreCache, err = corecache.NewKubeCoreCacheWithOptions(ctx, kubeClient, time.Hour, []string{testNamespace})
			Expect(err).NotTo(HaveOccurred())

			kubeSecret := helpers.GetKubeSecret(kubeSecretName, testNamespace)
			_, err = kubeClient.CoreV1().Secrets(testNamespace).Create(ctx,
				kubeSecret,
				metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			kubeSecretMatcher := gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"ObjectMeta": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name":      Equal(kubeSecretName),
					"Namespace": Equal(testNamespace),
				}),
			})
			Eventually(func(g Gomega) error {
				l, err := kubeClient.CoreV1().Secrets(testNamespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					return err
				}
				g.Expect(l.Items).To(ContainElement(kubeSecretMatcher))
				return nil
			}, "5s", "500ms").ShouldNot(HaveOccurred())
		}

		// setupVaultSecret will
		// - initiate vault instance
		// - create a new secret
		// - wait up to 5 seconds to confirm the existence of the secret
		//
		// as-is, this function is not idempotent and should be run only once
		setupVaultSecret := func() {
			vaultInstance = vaultFactory.MustVaultInstance()
			vaultInstance.Run(testCtx)

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

		getSettingsWithDefaults := func(setts *v1.Settings) *v1.Settings {
			if setts == nil {
				setts = &v1.Settings{}
			}
			setts.WatchNamespaces = []string{testNamespace}
			setts.RefreshRate = durationpb.New(time.Second)
			return setts
		}

		getSettingsForKubeSource := func(setts *v1.Settings) *v1.Settings {
			setts = getSettingsWithDefaults(setts)
			setts.SecretSource = &v1.Settings_KubernetesSecretSource{}
			return setts
		}

		getVaultSecrets := func() *v1.Settings_VaultSecrets {
			return &v1.Settings_VaultSecrets{
				Address: vaultInstance.Address(),
				AuthMethod: &v1.Settings_VaultSecrets_AccessToken{
					AccessToken: vaultInstance.Token(),
				},
			}
		}

		setVaultClientInitMap := func(idx int, vaultSettings *v1.Settings_VaultSecrets) {
			if vaultClientInitMap == nil {
				vaultClientInitMap = make(map[int]clients.VaultClientInitFunc)
			}
			vaultClientInitMap[idx] = func() *vaultapi.Client {
				c, err := clients.VaultClientForSettings(vaultSettings)
				Expect(err).NotTo(HaveOccurred())
				return c
			}
		}

		getSettingsForVaultSource := func(setts *v1.Settings) *v1.Settings {
			setts = getSettingsWithDefaults(setts)
			vaultSettings := getVaultSecrets()
			setts.SecretSource = &v1.Settings_VaultSecretSource{
				VaultSecretSource: vaultSettings,
			}
			setVaultClientInitMap(clients.SecretSourceAPIVaultClientInitIndex, vaultSettings)
			return setts
		}

		appendSource := func(sources []*v1.Settings_SecretOptions_Source, source *v1.Settings_SecretOptions_Source) []*v1.Settings_SecretOptions_Source {
			if sources == nil {
				sources = make([]*v1.Settings_SecretOptions_Source, 0)
			}
			return append(sources, source)
		}

		appendSourceToOptions := func(setts *v1.Settings, source *v1.Settings_SecretOptions_Source) *v1.Settings {
			secretOpts := setts.GetSecretOptions()
			if secretOpts == nil {
				secretOpts = &v1.Settings_SecretOptions{}
			}
			sources := appendSource(secretOpts.GetSources(), source)

			secretOpts.Sources = sources
			setts.SecretOptions = secretOpts
			return setts
		}

		getSettingsForKubeOptions := func(setts *v1.Settings) *v1.Settings {
			return appendSourceToOptions(
				getSettingsWithDefaults(setts),
				&v1.Settings_SecretOptions_Source{
					Source: &v1.Settings_SecretOptions_Source_Kubernetes{
						Kubernetes: &v1.Settings_KubernetesSecrets{},
					},
				})
		}

		getSettingsForVaultOptions := func(setts *v1.Settings) *v1.Settings {
			vaultSettings := getVaultSecrets()
			setts = appendSourceToOptions(
				getSettingsWithDefaults(setts),
				&v1.Settings_SecretOptions_Source{
					Source: &v1.Settings_SecretOptions_Source_Vault{
						Vault: vaultSettings,
					},
				})
			setVaultClientInitMap(len(setts.GetSecretOptions().GetSources())-1, vaultSettings)
			return setts
		}

		BeforeEach(func() {
			testCtx, testCancel = context.WithCancel(ctx)

			testNamespace = skhelpers.RandString(8)
		})

		AfterEach(func() {
			testCancel()
			settings = nil
		})

		JustBeforeEach(func() {
			factory, err := clients.SecretFactoryForSettings(ctx,
				clients.SecretFactoryParams{
					Settings:           settings,
					SharedCache:        nil,
					Cfg:                &cfg,
					Clientset:          &kubeClient,
					KubeCoreCache:      &kubeCoreCache,
					VaultClientInitMap: vaultClientInitMap,
					PluralName:         "secrets",
				})
			Expect(err).NotTo(HaveOccurred())
			secretClient, err = v1.NewSecretClient(ctx, factory)
			Expect(err).NotTo(HaveOccurred())
		})

		When("using secretSource API", func() {
			When("using a kubernetes secret source", func() {
				BeforeEach(func() {
					setupKubeSecret()
					settings = getSettingsForKubeSource(settings)
				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&kubesecret.ResourceClient{}))
					l, err := secretClient.List(testNamespace, skclients.ListOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(l).NotTo(BeNil())
					kubeSecret, err := l.Find(testNamespace, kubeSecretName)
					Expect(err).NotTo(HaveOccurred())
					Expect(kubeSecret).NotTo(BeNil())
				})
			})
			When("using a vault secret source", func() {
				BeforeEach(func() {
					setupVaultSecret()
					settings = getSettingsForVaultSource(settings)
				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&vault.ResourceClient{}))
					l, err := secretClient.List(testNamespace, skclients.ListOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(l).NotTo(BeNil())
					vaultSecret, err := l.Find(testNamespace, vaultSecretName)
					Expect(err).NotTo(HaveOccurred())
					Expect(vaultSecret).NotTo(BeNil())
				})
			})

		})
		When("using secretOptions API", func() {
			When("using a single kubernetes secret source", func() {
				BeforeEach(func() {
					setupKubeSecret()
					settings = getSettingsForKubeOptions(settings)
				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&kubesecret.ResourceClient{}))
					l, err := secretClient.List(testNamespace, skclients.ListOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(l).NotTo(BeNil())
					kubeSecret, err := l.Find(testNamespace, kubeSecretName)
					Expect(err).NotTo(HaveOccurred())
					Expect(kubeSecret).NotTo(BeNil())
				})
			})

			When("using a single vault secret source", func() {
				BeforeEach(func() {
					setupVaultSecret()
					settings = getSettingsForVaultOptions(settings)
				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&vault.ResourceClient{}))
					l, err := secretClient.List(testNamespace, skclients.ListOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(l).NotTo(BeNil())

					vaultSecret, err := l.Find(testNamespace, vaultSecretName)
					Expect(err).NotTo(HaveOccurred())
					Expect(vaultSecret).NotTo(BeNil(), fmt.Sprintf("%+v", l))
				})
			})
			When("using a kubernetes+vault secret source", func() {
				BeforeEach(func() {
					setupKubeSecret()
					setupVaultSecret()
					settings = getSettingsForKubeOptions(settings)
					settings = getSettingsForVaultOptions(settings)
				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&clients.MultiSecretResourceClient{}))
					l, err := secretClient.List(testNamespace, skclients.ListOpts{})
					Expect(err).NotTo(HaveOccurred())
					Expect(l).NotTo(BeNil())

					kubeSecret, err := l.Find(testNamespace, kubeSecretName)
					Expect(err).NotTo(HaveOccurred())
					Expect(kubeSecret).NotTo(BeNil())

					vaultSecret, err := l.Find(testNamespace, vaultSecretName)
					Expect(err).NotTo(HaveOccurred())
					Expect(vaultSecret).NotTo(BeNil(), fmt.Sprintf("%+v", l))
				})
			})
		})
	})
})
