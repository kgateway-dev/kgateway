package deployer

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gw2_v1alpha1 "github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"k8s.io/utils/ptr"
)

var _ = Describe("deepMergeGatewayParameters", func() {
	It("should override kube when selfManaged is set", func() {
		dst := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				Kube: &gw2_v1alpha1.KubernetesProxyConfig{},
			},
		}
		src := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				SelfManaged: &gw2_v1alpha1.SelfManagedGateway{},
			},
		}
		out := deepMergeGatewayParameters(dst, src)
		Expect(out).To(Equal(dst))
		Expect(out.Spec.Kube).To(Equal(src.Spec.Kube))
	})

	It("should override kube when selfManaged is unset", func() {
		dst := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				Kube: &gw2_v1alpha1.KubernetesProxyConfig{
					Deployment: &gw2_v1alpha1.ProxyDeployment{
						Replicas: ptr.To[uint32](2),
					},
				},
			},
		}
		src := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				Kube: &gw2_v1alpha1.KubernetesProxyConfig{
					Deployment: &gw2_v1alpha1.ProxyDeployment{
						Replicas: ptr.To[uint32](5),
					},
				},
			},
		}
		out := deepMergeGatewayParameters(dst, src)
		Expect(out).To(Equal(dst))
		Expect(out.Spec.Kube.Deployment.Replicas).To(Equal(src.Spec.Kube.Deployment.Replicas))
	})

	It("merges maps", func() {
		dst := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				Kube: &gw2_v1alpha1.KubernetesProxyConfig{
					PodTemplate: &gw2_v1alpha1.Pod{
						ExtraAnnotations: map[string]string{
							"a": "b",
						},
					},
					Service: &gw2_v1alpha1.Service{
						ExtraAnnotations: map[string]string{
							"c": "d",
						},
					},
					ServiceAccount: &gw2_v1alpha1.ServiceAccount{
						ExtraLabels: map[string]string{
							"e": "f",
						},
						ExtraAnnotations: map[string]string{
							"g": "h",
						},
					},
				},
			},
		}
		src := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				Kube: &gw2_v1alpha1.KubernetesProxyConfig{
					PodTemplate: &gw2_v1alpha1.Pod{
						ExtraAnnotations: map[string]string{
							"i": "j",
						},
					},
					Service: &gw2_v1alpha1.Service{
						ExtraAnnotations: map[string]string{
							"k": "l",
						},
					},
					ServiceAccount: &gw2_v1alpha1.ServiceAccount{
						ExtraLabels: map[string]string{
							"m": "n",
						},
						ExtraAnnotations: map[string]string{
							"o": "p",
						},
					},
				},
			},
		}
		out := deepMergeGatewayParameters(dst, src)
		Expect(out.Spec.Kube.PodTemplate.ExtraAnnotations).To(Equal(map[string]string{
			"a": "b",
			"i": "j",
		}))
		Expect(out.Spec.Kube.Service.ExtraAnnotations).To(Equal(map[string]string{
			"c": "d",
			"k": "l",
		}))
		Expect(out.Spec.Kube.ServiceAccount.ExtraLabels).To(Equal(map[string]string{
			"e": "f",
			"m": "n",
		}))
		Expect(out.Spec.Kube.ServiceAccount.ExtraAnnotations).To(Equal(map[string]string{
			"g": "h",
			"o": "p",
		}))
	})
})
