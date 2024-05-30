package deployer

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gw2_v1alpha1 "github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("deepMergeGatewayParameters", func() {
	It("should override kube when selfManaged is set", func() {
		dst := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				EnvironmentType: &gw2_v1alpha1.GatewayParametersSpec_Kube{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{},
				},
			},
		}
		src := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				EnvironmentType: &gw2_v1alpha1.GatewayParametersSpec_SelfManaged{
					SelfManaged: &emptypb.Empty{},
				},
			},
		}
		out := deepMergeGatewayParameters(dst, src)
		Expect(out).To(Equal(dst))
		Expect(out.Spec.GetEnvironmentType()).To(Equal(src.Spec.GetEnvironmentType()))
	})

	It("should override kube when selfManaged is unset", func() {
		dst := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				EnvironmentType: &gw2_v1alpha1.GatewayParametersSpec_Kube{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						WorkloadType: &gw2_v1alpha1.KubernetesProxyConfig_Deployment{
							Deployment: &gw2_v1alpha1.ProxyDeployment{
								Replicas: &wrappers.UInt32Value{Value: 2},
							},
						},
					},
				},
			},
		}
		src := &gw2_v1alpha1.GatewayParameters{
			Spec: gw2_v1alpha1.GatewayParametersSpec{
				EnvironmentType: &gw2_v1alpha1.GatewayParametersSpec_Kube{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						WorkloadType: &gw2_v1alpha1.KubernetesProxyConfig_Deployment{
							Deployment: &gw2_v1alpha1.ProxyDeployment{
								Replicas: &wrappers.UInt32Value{Value: 5},
							},
						},
					},
				},
			},
		}
		out := deepMergeGatewayParameters(dst, src)
		Expect(out).To(Equal(dst))
		Expect(out.Spec.GetKube().GetDeployment().GetReplicas()).To(Equal(src.Spec.GetKube().GetDeployment().GetReplicas()))
	})
})
