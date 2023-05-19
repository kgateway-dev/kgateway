package gloo_test

import (
	"context"
	"time"

	"github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/test/helpers"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Kubernetes tests for artifact client from projects/gloo/pkg/bootstrap
var _ = Describe("Artifact Client", func() {

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
		err := kube.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		cancel()
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
