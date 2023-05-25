package gloo_test

import (
	"time"

	"github.com/solo-io/solo-kit/test/helpers"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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

			testNamespace = helpers.RandString(8)
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
		var (
			vaultInstance *services.VaultInstance

			kubeSecretName string

			testNamespace string

			cfg           *rest.Config
			kubeClient    kubernetes.Interface
			kubeCoreCache corecache.KubeCoreCache

			vaultClientInitMap map[int]clients.VaultClientInitFunc

			secretClient v1.SecretClient
			settings     *v1.Settings
		)
		BeforeEach(func() {
			var err error
			testNamespace = helpers.RandString(8)
			_, err = kubeClient.CoreV1().Namespaces().Create(ctx, &kubev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespace,
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			kubeCoreCache, err = corecache.NewKubeCoreCacheWithOptions(ctx, kubeClient, time.Hour, []string{testNamespace})
			Expect(err).NotTo(HaveOccurred())

			kubeSecretName = "kubeSecret"

			_, err = kubeClient.CoreV1().Secrets(testNamespace).Create(ctx,
				&kubev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kubeSecretName,
						Namespace: testNamespace,
					},
				},
				metav1.CreateOptions{})

			settings = &v1.Settings{
				SecretOptions: &v1.Settings_SecretOptions{
					Sources: []*v1.Settings_SecretOptions_Source{
						{
							Source: &v1.Settings_SecretOptions_Source_Kubernetes{},
						},
					},
				},
				WatchNamespaces: []string{testNamespace},
			}

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

		When("using a single kubernetes secret source", func() {
			It("lists secrets", func() {
				l, err := secretClient.List(testNamespace, skclients.ListOpts{})
				Expect(err).NotTo(HaveOccurred())
				Expect(l).NotTo(BeNil())
				kubeSecret, err := l.Find(testNamespace, kubeSecretName)
				Expect(err).NotTo(HaveOccurred())
				Expect(kubeSecret).NotTo(BeNil())
			})
		})

		When("using a kubernetes+vault secret source", func() {
			var (
				vaultSecretName string
				secretForVault  *v1.Secret
			)

			BeforeEach(func() {
				vaultInstance = vaultFactory.MustVaultInstance()
				vaultInstance.Run(ctx)

				vaultSecretName = "vaultSecret"
				secretForVault = &v1.Secret{
					Kind: &v1.Secret_Tls{},
					Metadata: &core.Metadata{
						Name:      vaultSecretName,
						Namespace: testNamespace,
					},
				}

				vaultInstance.WriteSecret(secretForVault)

				vaultSettings := &v1.Settings_VaultSecrets{
					Address: vaultInstance.Address(),
					AuthMethod: &v1.Settings_VaultSecrets_AccessToken{
						AccessToken: vaultInstance.Token(),
					},
				}

				sources := settings.GetSecretOptions().GetSources()
				sources = append(sources, &v1.Settings_SecretOptions_Source{
					Source: &v1.Settings_SecretOptions_Source_Vault{
						Vault: vaultSettings,
					},
				})

				vaultClientInitMap = map[int]clients.VaultClientInitFunc{
					1: func() *vaultapi.Client {
						c, err := clients.VaultClientForSettings(vaultSettings)
						Expect(err).NotTo(HaveOccurred())
						return c
					},
				}
			})
			It("lists secrets", func() {
				l, err := secretClient.List(testNamespace, skclients.ListOpts{})
				Expect(err).NotTo(HaveOccurred())
				Expect(l).NotTo(BeNil())

				kubeSecret, err := l.Find(testNamespace, kubeSecretName)
				Expect(err).NotTo(HaveOccurred())
				Expect(kubeSecret).NotTo(BeNil())

				vaultSecret, err := l.Find(testNamespace, vaultSecretName)
				Expect(err).NotTo(HaveOccurred())
				Expect(vaultSecret).NotTo(BeNil())
			})
		})
	})
})
