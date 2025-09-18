package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	gw2_v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func TestDeepMergeGatewayParameters(t *testing.T) {
	tests := []struct {
		name string
		dst  *gw2_v1alpha1.GatewayParameters
		src  *gw2_v1alpha1.GatewayParameters
		want *gw2_v1alpha1.GatewayParameters
		// Add a validation function that can perform additional checks
		validate func(t *testing.T, got *gw2_v1alpha1.GatewayParameters)
	}{
		{
			name: "should override kube when selfManaged is set",
			dst: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{},
				},
			},
			src: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					SelfManaged: &gw2_v1alpha1.SelfManagedGateway{},
				},
			},
			want: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube:        nil,
					SelfManaged: &gw2_v1alpha1.SelfManagedGateway{},
				},
			},
		},
		{
			name: "should override kube deployment replicas",
			dst: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						Deployment: &gw2_v1alpha1.ProxyDeployment{
							Replicas: ptr.To[uint32](2),
						},
					},
				},
			},
			src: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						Deployment: &gw2_v1alpha1.ProxyDeployment{
							Replicas: ptr.To[uint32](5),
						},
					},
				},
			},
			want: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						Deployment: &gw2_v1alpha1.ProxyDeployment{
							Replicas: ptr.To[uint32](5),
						},
					},
				},
			},
		},
		{
			name: "should override kube deployment omitReplicas",
			dst: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						Deployment: &gw2_v1alpha1.ProxyDeployment{
							Replicas: ptr.To[uint32](2),
						},
					},
				},
			},
			src: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						Deployment: &gw2_v1alpha1.ProxyDeployment{
							OmitReplicas: ptr.To(true),
						},
					},
				},
			},
			want: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						Deployment: &gw2_v1alpha1.ProxyDeployment{
							OmitReplicas: ptr.To(true),
						},
					},
				},
			},
		},
		{
			name: "merges maps",
			dst: &gw2_v1alpha1.GatewayParameters{
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
			},
			src: &gw2_v1alpha1.GatewayParameters{
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
			},
			want: &gw2_v1alpha1.GatewayParameters{
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						PodTemplate: &gw2_v1alpha1.Pod{
							ExtraLabels: map[string]string{
								"a": "aaa-override",
								"b": "bbb",
								"c": "ccc",
							},
							ExtraAnnotations: map[string]string{
								"a": "aaa-override",
								"b": "bbb",
								"c": "ccc",
							},
						},
						Service: &gw2_v1alpha1.Service{
							ExtraLabels: map[string]string{
								"a": "aaa-override",
								"b": "bbb",
								"c": "ccc",
							},
							ExtraAnnotations: map[string]string{
								"a": "aaa-override",
								"b": "bbb",
								"c": "ccc",
							},
						},
						ServiceAccount: &gw2_v1alpha1.ServiceAccount{
							ExtraLabels: map[string]string{
								"a": "aaa-override",
								"b": "bbb",
								"c": "ccc",
							},
							ExtraAnnotations: map[string]string{
								"a": "aaa-override",
								"b": "bbb",
								"c": "ccc",
							},
						},
					},
				},
			},
			validate: func(t *testing.T, got *gw2_v1alpha1.GatewayParameters) {
				expectedMap := map[string]string{
					"a": "aaa-override",
					"b": "bbb",
					"c": "ccc",
				}
				assert.Equal(t, expectedMap, got.Spec.Kube.PodTemplate.ExtraLabels)
				assert.Equal(t, expectedMap, got.Spec.Kube.PodTemplate.ExtraAnnotations)
				assert.Equal(t, expectedMap, got.Spec.Kube.Service.ExtraLabels)
				assert.Equal(t, expectedMap, got.Spec.Kube.Service.ExtraAnnotations)
				assert.Equal(t, expectedMap, got.Spec.Kube.ServiceAccount.ExtraLabels)
				assert.Equal(t, expectedMap, got.Spec.Kube.ServiceAccount.ExtraAnnotations)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeepMergeGatewayParameters(tt.dst, tt.src)
			assert.Equal(t, tt.want, got)

			// Run additional validation if provided
			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}
