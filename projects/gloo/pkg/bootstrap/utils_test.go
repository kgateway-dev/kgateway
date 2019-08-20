package bootstrap_test

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
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

})
