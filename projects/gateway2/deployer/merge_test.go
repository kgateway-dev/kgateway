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
						ExtraLabels: map[string]string{
							"a": "aaa",
							"b": "bbb",
						},
						ExtraAnnotations: map[string]string{
							"a": "aaa",
							"b": "bbb",
						},
					},
					Service: &gw2_v1alpha1.Service{
						ExtraLabels: map[string]string{
							"a": "aaa",
							"b": "bbb",
						},
						ExtraAnnotations: map[string]string{
							"a": "aaa",
							"b": "bbb",
						},
					},
					ServiceAccount: &gw2_v1alpha1.ServiceAccount{
						ExtraLabels: map[string]string{
							"a": "aaa",
							"b": "bbb",
						},
						ExtraAnnotations: map[string]string{
							"a": "aaa",
							"b": "bbb",
						},
					},
				},
			},
		}
		src := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				Kube: &gw2_v1alpha1.KubernetesProxyConfig{
					PodTemplate: &gw2_v1alpha1.Pod{
						ExtraLabels: map[string]string{
							"a": "aaa-override",
							"c": "ccc",
						},
						ExtraAnnotations: map[string]string{
							"a": "aaa-override",
							"c": "ccc",
						},
					},
					Service: &gw2_v1alpha1.Service{
						ExtraLabels: map[string]string{
							"a": "aaa-override",
							"c": "ccc",
						},
						ExtraAnnotations: map[string]string{
							"a": "aaa-override",
							"c": "ccc",
						},
					},
					ServiceAccount: &gw2_v1alpha1.ServiceAccount{
						ExtraLabels: map[string]string{
							"a": "aaa-override",
							"c": "ccc",
						},
						ExtraAnnotations: map[string]string{
							"a": "aaa-override",
							"c": "ccc",
						},
					},
				},
			},
		}
		out := deepMergeGatewayParameters(dst, src)
		expectedMap := map[string]string{
			"a": "aaa-override",
			"b": "bbb",
			"c": "ccc",
		}
		Expect(out.Spec.Kube.PodTemplate.ExtraLabels).To(Equal(expectedMap))
		Expect(out.Spec.Kube.PodTemplate.ExtraAnnotations).To(Equal(expectedMap))
		Expect(out.Spec.Kube.Service.ExtraLabels).To(Equal(expectedMap))
		Expect(out.Spec.Kube.Service.ExtraAnnotations).To(Equal(expectedMap))
		Expect(out.Spec.Kube.ServiceAccount.ExtraLabels).To(Equal(expectedMap))
		Expect(out.Spec.Kube.ServiceAccount.ExtraAnnotations).To(Equal(expectedMap))
	})
})
