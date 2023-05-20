package clients_test

import (
	"context"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/hashicorp/consul/api"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
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
