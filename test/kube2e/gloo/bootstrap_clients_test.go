package gloo_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector/kube"
	"github.com/solo-io/gloo/test/kube2e/helper"
	kubetestclients "github.com/solo-io/gloo/test/kubernetes/testutils/clients"

	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/vault"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
	corev1 "k8s.io/api/core/v1"

	"github.com/hashicorp/consul/api"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	skclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/gomega"

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

			cfg = kubetestclients.MustRestConfig()
			kubeClient = resourceClientset.KubeClients()

			testNamespace = skhelpers.RandString(8)
			_, err = kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
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
					&corev1.ConfigMap{
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
			cfg = kubetestclients.MustRestConfig()
			kubeClient = resourceClientset.KubeClients()

			_, err = kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
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

		getVaultSecrets := func(vi *services.VaultInstance) *v1.Settings_VaultSecrets {
			return &v1.Settings_VaultSecrets{
				Address: vi.Address(),
				AuthMethod: &v1.Settings_VaultSecrets_AccessToken{
					AccessToken: vi.Token(),
				},
			}
		}

		setVaultClientInitMap := func(idx int, vaultSettings *v1.Settings_VaultSecrets) {
			vaultClientInitMap[idx] = func(ctx context.Context) *vaultapi.Client {
				c, err := clients.VaultClientForSettings(ctx, vaultSettings)
				Expect(err).NotTo(HaveOccurred())
				return c
			}
		}

		appendSourceToOptions := func(source *v1.Settings_SecretOptions_Source) {
			secretOpts := settings.GetSecretOptions()
			if secretOpts == nil {
				secretOpts = &v1.Settings_SecretOptions{}
			}
			sources := secretOpts.GetSources()
			if sources == nil {
				sources = make([]*v1.Settings_SecretOptions_Source, 0)
			}
			sources = append(sources, source)

			secretOpts.Sources = sources
			settings.SecretOptions = secretOpts
		}

		BeforeEach(func() {
			testCtx, testCancel = context.WithCancel(ctx)

			testNamespace = skhelpers.RandString(8)
			settings = &v1.Settings{
				WatchNamespaces: []string{testNamespace},
			}
			vaultClientInitMap = make(map[int]clients.VaultClientInitFunc)
		})

		AfterEach(func() {
			testCancel()
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

		listSecret := func(g Gomega, secretName string) {
			l, err := secretClient.List(testNamespace, skclients.ListOpts{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(l).NotTo(BeNil())
			kubeSecret, err := l.Find(testNamespace, secretName)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kubeSecret).NotTo(BeNil())
		}

		When("using secretSource API", func() {
			When("using a kubernetes secret source", func() {
				BeforeEach(func() {
					setupKubeSecret()
					settings.SecretSource = &v1.Settings_KubernetesSecretSource{}
				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&kubesecret.ResourceClient{}))
					Eventually(func(g Gomega) {
						listSecret(g, kubeSecretName)
					}, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Succeed())
				})
			})
			When("using a vault secret source", func() {
				BeforeEach(func() {
					setupVaultSecret()
					vaultSettings := getVaultSecrets(vaultInstance)
					settings.SecretSource = &v1.Settings_VaultSecretSource{
						VaultSecretSource: vaultSettings,
					}
					setVaultClientInitMap(clients.SecretSourceAPIVaultClientInitIndex, vaultSettings)
				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&vault.ResourceClient{}))
					Eventually(func(g Gomega) {
						listSecret(g, vaultSecretName)
					}, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Succeed())
				})
			})

		})
		When("using secretOptions API", func() {
			When("using a single kubernetes secret source", func() {
				BeforeEach(func() {
					setupKubeSecret()
					appendSourceToOptions(
						&v1.Settings_SecretOptions_Source{
							Source: &v1.Settings_SecretOptions_Source_Kubernetes{
								Kubernetes: &v1.Settings_KubernetesSecrets{},
							},
						})
				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&kubesecret.ResourceClient{}))
					Eventually(func(g Gomega) {
						listSecret(g, kubeSecretName)
					}, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Succeed())
				})
			})

			When("using a single vault secret source", func() {
				BeforeEach(func() {
					setupVaultSecret()
					vaultSettings := getVaultSecrets(vaultInstance)
					appendSourceToOptions(
						&v1.Settings_SecretOptions_Source{
							Source: &v1.Settings_SecretOptions_Source_Vault{
								Vault: vaultSettings,
							},
						})
					setVaultClientInitMap(len(settings.GetSecretOptions().GetSources())-1, vaultSettings)

				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&vault.ResourceClient{}))
					Eventually(func(g Gomega) {
						listSecret(g, vaultSecretName)
					}, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Succeed())
				})
			})
			When("using a kubernetes+vault secret source", func() {
				BeforeEach(func() {
					setupKubeSecret()
					appendSourceToOptions(
						&v1.Settings_SecretOptions_Source{
							Source: &v1.Settings_SecretOptions_Source_Kubernetes{
								Kubernetes: &v1.Settings_KubernetesSecrets{},
							},
						})

					setupVaultSecret()
					vaultSettings := getVaultSecrets(vaultInstance)
					appendSourceToOptions(
						&v1.Settings_SecretOptions_Source{
							Source: &v1.Settings_SecretOptions_Source_Vault{
								Vault: vaultSettings,
							},
						})
					setVaultClientInitMap(len(settings.GetSecretOptions().GetSources())-1, vaultSettings)
				})
				It("lists secrets", func() {
					Expect(secretClient.BaseClient()).To(BeAssignableToTypeOf(&clients.MultiSecretResourceClient{}))
					Eventually(func(g Gomega) {
						listSecret(g, kubeSecretName)
					}, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Succeed())
					Eventually(func(g Gomega) {
						listSecret(g, vaultSecretName)
					}, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Succeed())
				})
			})
		})
	})

	Context("Retry leader election failure", func() {

		var deploymentClient clientsv1.DeploymentInterface
		var verifyTranslation func()

		BeforeEach(func() {
			deploymentClient = resourceClientset.KubeClients().AppsV1().Deployments(testHelper.InstallNamespace)

			// verifyTranslation creates a VS with a directActionRoute and verifies it has been accepted
			// and translated by curling against the route specified route
			verifyTranslation = func() {
				name := "test-vs"
				domain := "test-vs-domain"
				path := "/test"
				response := "OK"

				testVS := helpers.NewVirtualServiceBuilder().
					WithName(name).
					WithNamespace(testHelper.InstallNamespace).
					WithDomain(domain).
					WithRoutePrefixMatcher("test", path).
					WithRouteDirectResponseAction("test", &v1.DirectResponseAction{
						Status: 200,
						Body:   response,
					}).
					Build()

				resourceClientset.VirtualServiceClient().Write(testVS, skclients.WriteOpts{})
				// Since the kube api server can be down when the VS is written,
				// specify a long enough interval for it to be accepted when the kube api server comes back up
				helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					return resourceClientset.VirtualServiceClient().Read(testHelper.InstallNamespace, testVS.Metadata.Name, skclients.ReadOpts{})
				}, "120s", "10s")
				defer resourceClientset.VirtualServiceClient().Delete(testHelper.InstallNamespace, testVS.Metadata.Name, skclients.DeleteOpts{})

				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Method:            "GET",
					Path:              path,
					Host:              domain,
					Service:           gatewaydefaults.GatewayProxyName,
					Port:              gatewayPort,
					ConnectionTimeout: 1,
					WithoutStats:      true,
				}, response, 1, 60*time.Second, 1*time.Second)
			}
		})

		AfterEach(func() {
			testHelper.ModifyDeploymentEnv(ctx, deploymentClient, testHelper.InstallNamespace, "gloo", 0, corev1.EnvVar{
				Name:  kube.MaxRecoveryDurationWithoutKubeAPIServer,
				Value: "",
			})
		})

		It("does not recover by default", func() {
			waitUntilStartsLeading()
			simulateKubeAPIServerUnavailability()

			// By default it should crash
			Eventually(func(g Gomega) {
				logs := testHelper.GetContainerLogs(ctx, testHelper.InstallNamespace, "deploy/gloo")
				g.Expect(logs).To(ContainSubstring("lost leadership, quitting app"))
			}, "30s", "1s").Should(Succeed())

			verifyTranslation()
		})

		It("recovers when MAX_RECOVERY_DURATION_WITHOUT_KUBE_API_SERVER is set", func() {
			testHelper.ModifyDeploymentEnv(ctx, deploymentClient, testHelper.InstallNamespace, "gloo", 0, corev1.EnvVar{
				Name: kube.MaxRecoveryDurationWithoutKubeAPIServer,
				// This invalid value will test whether it falls back to the default value of 60s
				Value: "abcd",
			})

			// Since a new deployment has been rolled out by changing the MAX_RECOVERY_DURATION_WITHOUT_KUBE_API_SERVER env var,
			// we can be sure that the logs fetched were generated only after this test has begun
			waitUntilStartsLeading()

			// Simulate the kube api server is down and bring it up after 15 seconds. While it is down :
			// - The leader should lose the lease
			// - Create a VS
			// When the kube api server is back up :
			// - The VS should be accepted
			// - It should be translated into an envoy config
			restoreKubeAPIServer := simulateKubeAPIServerDown()
			go func() {
				time.Sleep(15 * time.Second)
				restoreKubeAPIServer()
			}()

			// This creates a VS and ensures that it is accepted and translated. Run this while the kube api server is down to verify that
			// reports are written once the pod recovers and becomes a leader
			verified := make(chan struct{})
			go func() {
				verifyTranslation()
				verified <- struct{}{}
			}()

			Eventually(func(g Gomega) {
				logs := testHelper.GetContainerLogs(ctx, testHelper.InstallNamespace, "deploy/gloo")
				g.Expect(logs).To(ContainSubstring(fmt.Sprintf("%s is not a valid duration. Defaulting to 60s", kube.MaxRecoveryDurationWithoutKubeAPIServer)))
				g.Expect(logs).To(ContainSubstring("Leader election cycle 0 lost. Trying again"))
				g.Expect(logs).To(ContainSubstring("recovered from lease renewal failure"))
				g.Expect(logs).NotTo(ContainSubstring("lost leadership, quitting app"))
			}, "60s", "1s").Should(Succeed())

			// Wait for the goroutine to finish
			<-verified
		})

		// During this test :
		// - Scale up the deployment to 2 pods
		// - Block kube api access to the leader pod
		// - Verify the leader pod loses leadership
		// - Verify the second pod becomes the leader
		// - Create a resource and verify it has been translated
		It("concedes leadership to another pod", func() {
			testHelper.ModifyDeploymentEnv(ctx, deploymentClient, testHelper.InstallNamespace, "gloo", 0, corev1.EnvVar{
				Name:  kube.MaxRecoveryDurationWithoutKubeAPIServer,
				Value: "45s",
			})

			// Since a new deployment has been rolled out by changing the MAX_RECOVERY_DURATION_WITHOUT_KUBE_API_SERVER env var,
			// we can be sure that the logs fetched were generated only after this test had begun
			waitUntilStartsLeading()

			// Get the leader pod. Since there is only one pod it is the leader
			leader, _, err := testHelper.Execute(ctx, "get", "pods", "-n", testHelper.InstallNamespace, "-l", "gloo=gloo", "-o", "jsonpath='{.items[0].metadata.name}'")
			Expect(err).ToNot(HaveOccurred())
			leader = strings.ReplaceAll(leader, "'", "")

			// Scale the deployment to 2 replicas so the other can take over when the leader is unable to communicate with the Kube API server
			err = testHelper.Scale(ctx, testHelper.InstallNamespace, "deploy/gloo", 2)
			Expect(err).ToNot(HaveOccurred())
			defer func() {
				err = testHelper.Scale(ctx, testHelper.InstallNamespace, "deploy/gloo", 1)
				Expect(err).ToNot(HaveOccurred())
			}()

			// Get the follower pod.
			follower, _, err := testHelper.Execute(ctx, "get", "pods", "-n", testHelper.InstallNamespace, "-l", "gloo=gloo", "-o", "jsonpath='{.items[*].metadata.name}'")
			Expect(err).ToNot(HaveOccurred())
			// Before: 'gloo-7bd4788f8c-6qvd8' (leader) 'gloo-7bd4788f8c-qdjn6' (follower)
			// After: gloo-7bd4788f8c-qdjn6 (follower)
			follower = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(follower, "'", ""), " ", ""), leader, "")

			// Verify that the follower is indeed not leading
			Eventually(func(g Gomega) {
				logs := testHelper.GetContainerLogs(ctx, testHelper.InstallNamespace, "pod/"+follower)
				g.Expect(logs).To(ContainSubstring("new leader elected with ID: " + leader))
				g.Expect(logs).ToNot(ContainSubstring("starting leadership"))
			}, "60s", "1s").Should(Succeed())

			// Label the leader so the network policy can block communication to the Kube API server
			pod, err := resourceClientset.KubeClients().CoreV1().Pods(testHelper.InstallNamespace).Get(ctx, leader, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			pod.Labels["block"] = "this"
			_, err = resourceClientset.KubeClients().CoreV1().Pods(testHelper.InstallNamespace).Update(ctx, pod, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Block the leader's communication to the Kube API server
			err = testHelper.ApplyFile(ctx, testHelper.RootDir+"/test/kube2e/gloo/artifacts/block-labels.yaml")
			Expect(err).ToNot(HaveOccurred())

			// Verify that the leader has stopped leading
			Eventually(func(g Gomega) {
				logs := testHelper.GetContainerLogs(ctx, testHelper.InstallNamespace, "pod/"+leader)
				g.Expect(logs).To(ContainSubstring("max recovery from kube apiserver unavailability set to 45s"))
				g.Expect(logs).To(ContainSubstring("lost leadership"))
			}, "600s", "10s").Should(Succeed())

			// Verify that the follower has become the new leader
			Eventually(func(g Gomega) {
				logs := testHelper.GetContainerLogs(ctx, testHelper.InstallNamespace, "pod/"+follower)
				g.Expect(logs).To(ContainSubstring("starting leadership"))
			}, "60s", "1s").Should(Succeed())

			// Cleanup the network policy.
			err = testHelper.DeleteFile(ctx, testHelper.RootDir+"/test/kube2e/gloo/artifacts/block-labels.yaml")
			Expect(err).ToNot(HaveOccurred())

			// With connectivity restored, the old leader can become a follower
			// Verify that the old leader has become a follower
			Eventually(func(g Gomega) {
				logs := testHelper.GetContainerLogs(ctx, testHelper.InstallNamespace, "pod/"+leader)
				g.Expect(logs).To(ContainSubstring("recovered from lease renewal failure"))
				g.Expect(logs).To(ContainSubstring("new leader elected with ID: " + follower))
			}, "60s", "1s").Should(Succeed())

			// Ensure that we can still operate.
			verifyTranslation()
		})
	})
})

// simulateKubeAPIServerDown blocks network connectivity between the gloo pod and the kube api server.
// It returns a function that restores network connectivity.
func simulateKubeAPIServerDown() func() {
	err := testHelper.ApplyFile(ctx, testHelper.RootDir+"/test/kube2e/gloo/artifacts/block-gloo-apiserver.yaml")
	Expect(err).ToNot(HaveOccurred())

	return func() {
		err := testHelper.DeleteFile(ctx, testHelper.RootDir+"/test/kube2e/gloo/artifacts/block-gloo-apiserver.yaml")
		Expect(err).ToNot(HaveOccurred())
	}
}

// simulateKubeAPIServerUnavailability temporarily blocks network connectivity between the gloo pod and the kube api server
func simulateKubeAPIServerUnavailability() {
	restoreKubeAPIServer := simulateKubeAPIServerDown()
	time.Sleep(15 * time.Second)
	restoreKubeAPIServer()
}

func waitUntilStartsLeading() {
	// Initially sleep as the new deployment might be rolling out
	time.Sleep(10 * time.Second)
	Eventually(func(g Gomega) {
		out := testHelper.GetContainerLogs(ctx, testHelper.InstallNamespace, "deploy/gloo")
		g.Expect(out).To(ContainSubstring("starting leadership"))
	}, "120s", "10s").Should(Succeed())
}
