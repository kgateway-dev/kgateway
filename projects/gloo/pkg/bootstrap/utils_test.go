package bootstrap_test

import (
	"context"
	"os"
	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/test/helpers"
	"github.com/solo-io/solo-kit/test/setup"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
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

	Context("kube converter", func() {
		It("should convert config map to artifact", func() {
			kubeConverter := NewKubeConverter()
			cfgmap := &kubev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cfg", Namespace: "foo",
					Labels:      map[string]string{"foo": "bar"},
					Annotations: map[string]string{"foo": "bar2"},
				},
				Data: map[string]string{
					"test": "data",
				},
			}
			artifact, err := kubeConverter.FromKubeConfigMapWithResource(context.TODO(), new(v1.Artifact), "artifact", cfgmap)
			Expect(err).NotTo(HaveOccurred())
			Expect(artifact).NotTo(BeNil())
			cfgmap2, err := kubeConverter.ToKubeConfigMapSimple(context.TODO(), artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfgmap).To(Equal(cfgmap2))

		})
	})

	Context("kube tests", func() {
		BeforeEach(func() {
			if os.Getenv("RUN_KUBE_TESTS") != "1" {
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
			_, err = kube.CoreV1().Namespaces().Create(&kubev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			})
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
				_, err := kube.CoreV1().ConfigMaps(namespace).Create(
					&kubev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg"},
						Data: map[string]string{
							"test": "data",
						},
					})
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
					"artifacts")
				Expect(err).NotTo(HaveOccurred())
				artifactClient, err = v1.NewArtifactClient(factory)
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
})
