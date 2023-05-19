package bootstrap_test

import (
	"context"
	"time"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/hashicorp/consul/api"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/test/helpers"
	"github.com/solo-io/solo-kit/test/setup"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var _ = Describe("Utils", func() {

	It("should set kube rate limts", func() {
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
		params := NewConfigFactoryParams(
			settings,
			nil,
			nil,
			&cfg,
			nil,
		)

		kubefactory, err := ConfigFactoryForSettings(params, v1.UpstreamCrd)

		Expect(err).ToNot(HaveOccurred())
		Expect(cfg).ToNot(BeNil())
		Expect(kubefactory.(*factory.KubeResourceClientFactory).Cfg).To(Equal(cfg))

		Expect(cfg.QPS).To(Equal(float32(100.5)))
		Expect(cfg.Burst).To(Equal(1000))
	})

	Context("kube tests", func() {
		BeforeEach(func() {
			if !testutils.ShouldRunKubeTests() {
				Skip("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
			}
		})

		var (
			namespace     string
			cfg           *rest.Config
			ctx           context.Context
			cancel        context.CancelFunc
			kube          kubernetes.Interface
			kubeCoreCache corecache.KubeCoreCache
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			namespace = helpers.RandString(8)
			var err error
			cfg, err = kubeutils.GetConfig("", "")
			Expect(err).NotTo(HaveOccurred())

			kube, err = kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			_, err = kube.CoreV1().Namespaces().Create(ctx, &kubev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			kubeCoreCache, err = corecache.NewKubeCoreCacheWithOptions(ctx, kube, time.Hour, []string{namespace})
			Expect(err).NotTo(HaveOccurred())

		})
		AfterEach(func() {
			cancel()
			setup.TeardownKube(namespace)
		})

		Context("artifacts as config maps", func() {

			var (
				artifactClient v1.ArtifactClient
			)

			BeforeEach(func() {
				_, err := kube.CoreV1().ConfigMaps(namespace).Create(ctx,
					&kubev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg"},
						Data: map[string]string{
							"test": "data",
						},
					}, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				settings := &v1.Settings{
					ArtifactSource:  &v1.Settings_KubernetesArtifactSource{},
					WatchNamespaces: []string{namespace},
				}

				factory, err := ArtifactFactoryForSettings(ctx,
					settings,
					nil,
					&cfg,
					&kube,
					&kubeCoreCache,
					&api.Client{},
					"artifacts")
				Expect(err).NotTo(HaveOccurred())
				artifactClient, err = v1.NewArtifactClient(ctx, factory)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should work with artifacts", func() {
				artifact, err := artifactClient.Read(namespace, "cfg", clients.ReadOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
				Expect(artifact.GetMetadata().Name).To(Equal("cfg"))
				Expect(artifact.Data["test"]).To(Equal("data"))
			})
		})
	})

	Context("consul tests", func() {

		var (
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {

			if !testutils.IsEnvTruthy(testutils.RunConsulTests) {
				Skip("This test downloads and runs consul and is disabled by default. To enable, set RUN_CONSUL_TESTS=1 in your env.")
				return
			}

			ctx, cancel = context.WithCancel(context.Background())

			consulInstance = consulFactory.MustConsulInstance()
			err := consulInstance.Run(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cancel()
		})

		Context("artifacts as consul key value", func() {

			var (
				ctx            context.Context
				cancel         func()
				artifactClient v1.ArtifactClient
			)

			BeforeEach(func() {
				ctx, cancel = context.WithCancel(context.Background())

				value, err := protoutils.MarshalYAML(&v1.Artifact{
					Data:     map[string]string{"hi": "bye"},
					Metadata: &core.Metadata{Name: "name", Namespace: "namespace"},
				})
				Expect(err).NotTo(HaveOccurred())

				_, err = client.KV().Put(&api.KVPair{
					Key:   "gloo/gloo.solo.io/v1/Artifact/namespace/name",
					Value: value,
				}, nil)
				Expect(err).NotTo(HaveOccurred())

				settings := &v1.Settings{
					ArtifactSource: &v1.Settings_ConsulKvArtifactSource{
						ConsulKvArtifactSource: &v1.Settings_ConsulKv{
							RootKey: "gloo",
						},
					},
				}

				factory, err := ArtifactFactoryForSettings(ctx,
					settings,
					nil,
					nil,
					nil,
					nil,
					client,
					"artifacts")
				Expect(err).NotTo(HaveOccurred())
				artifactClient, err = v1.NewArtifactClient(ctx, factory)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				cancel()
			})

			It("should work with artifacts", func() {
				artifact, err := artifactClient.Read("namespace", "name", clients.ReadOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
				Expect(artifact.GetMetadata().Name).To(Equal("name"))
				Expect(artifact.Data["hi"]).To(Equal("bye"))
			})
		})
	})
})
