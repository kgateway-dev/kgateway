package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestDeepMergeGatewayParameters(t *testing.T) {
	tests := []struct {
		name string
		dst  *kgateway.GatewayParameters
		src  *kgateway.GatewayParameters
		want *kgateway.GatewayParameters
		// Add a validation function that can perform additional checks
		validate func(t *testing.T, got *kgateway.GatewayParameters)
	}{
		{
			name: "should override kube when selfManaged is set",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					SelfManaged: &kgateway.SelfManagedGateway{},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube:        nil,
					SelfManaged: &kgateway.SelfManagedGateway{},
				},
			},
		},
		{
			name: "should override kube deployment replicas by default",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: ptr.To[int32](5),
						},
					},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: ptr.To[int32](5),
						},
					},
				},
			},
		},
		{
			name: "should override kube deployment replicas if explicit",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: ptr.To[int32](2),
						},
					},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: ptr.To[int32](3),
						},
					},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: ptr.To[int32](3),
						},
					},
				},
			},
		},
		{
			name: "should not override kube deployment replicas if src is nil",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: ptr.To[int32](2),
						},
					},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: ptr.To[int32](2),
						},
					},
				},
			},
		},
		{
			name: "merges maps",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						PodTemplate: &kgateway.Pod{
							ExtraLabels: map[string]string{
								"a": "aaa",
								"b": "bbb",
							},
							ExtraAnnotations: map[string]string{
								"a": "aaa",
								"b": "bbb",
							},
						},
						Service: &kgateway.Service{
							ExtraLabels: map[string]string{
								"a": "aaa",
								"b": "bbb",
							},
							ExtraAnnotations: map[string]string{
								"a": "aaa",
								"b": "bbb",
							},
						},
						ServiceAccount: &kgateway.ServiceAccount{
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
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						PodTemplate: &kgateway.Pod{
							ExtraLabels: map[string]string{
								"a": "aaa-override",
								"c": "ccc",
							},
							ExtraAnnotations: map[string]string{
								"a": "aaa-override",
								"c": "ccc",
							},
						},
						Service: &kgateway.Service{
							ExtraLabels: map[string]string{
								"a": "aaa-override",
								"c": "ccc",
							},
							ExtraAnnotations: map[string]string{
								"a": "aaa-override",
								"c": "ccc",
							},
						},
						ServiceAccount: &kgateway.ServiceAccount{
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
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						PodTemplate: &kgateway.Pod{
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
						Service: &kgateway.Service{
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
						ServiceAccount: &kgateway.ServiceAccount{
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
			validate: func(t *testing.T, got *kgateway.GatewayParameters) {
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
		{
			name: "merges envoy extra args",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						EnvoyContainer: &kgateway.EnvoyContainer{
							ExtraArgs: []string{"--base-id", "1"},
						},
					},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						EnvoyContainer: &kgateway.EnvoyContainer{
							ExtraArgs: []string{"--disable-extensions", "envoy.filters.http.lua"},
						},
					},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						EnvoyContainer: &kgateway.EnvoyContainer{
							ExtraArgs: []string{"--base-id", "1", "--disable-extensions", "envoy.filters.http.lua"},
						},
					},
				},
			},
		},
		{
			name: "should have only one probeHandler action",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						PodTemplate: &kgateway.Pod{
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"exec", "command"},
									},
								},
							},
						},
					},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						PodTemplate: &kgateway.Pod{
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromString("8080"),
									},
								},
							},
						},
					},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						PodTemplate: &kgateway.Pod{
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromString("8080"),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "should merge the default probeHandler action if none specified",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						PodTemplate: &kgateway.Pod{
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"exec", "command"},
									},
								},
							},
						},
					},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						PodTemplate: &kgateway.Pod{
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"exec", "command"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "should merge service loadBalancerClass from src",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Service: &kgateway.Service{
							Type: ptr.To(corev1.ServiceTypeLoadBalancer),
						},
					},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Service: &kgateway.Service{
							LoadBalancerClass: new("service.k8s.aws/nlb"),
						},
					},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Service: &kgateway.Service{
							Type:              ptr.To(corev1.ServiceTypeLoadBalancer),
							LoadBalancerClass: new("service.k8s.aws/nlb"),
						},
					},
				},
			},
		},
		{
			name: "should override service loadBalancerClass from src",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Service: &kgateway.Service{
							Type:              ptr.To(corev1.ServiceTypeLoadBalancer),
							LoadBalancerClass: new("default-class"),
						},
					},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Service: &kgateway.Service{
							LoadBalancerClass: new("service.k8s.aws/nlb"),
						},
					},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Service: &kgateway.Service{
							Type:              ptr.To(corev1.ServiceTypeLoadBalancer),
							LoadBalancerClass: new("service.k8s.aws/nlb"),
						},
					},
				},
			},
		},
		{
			name: "should not override service loadBalancerClass if src is nil",
			dst: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Service: &kgateway.Service{
							Type:              ptr.To(corev1.ServiceTypeLoadBalancer),
							LoadBalancerClass: new("default-class"),
						},
					},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Service: &kgateway.Service{},
					},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Service: &kgateway.Service{
							Type:              ptr.To(corev1.ServiceTypeLoadBalancer),
							LoadBalancerClass: new("default-class"),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			DeepMergeGatewayParameters(tt.dst, tt.src)
			assert.Equal(t, tt.want, tt.dst)

			// Run additional validation if provided
			if tt.validate != nil {
				tt.validate(t, tt.dst)
			}
		})
	}
}

func TestDeepMergeImage(t *testing.T) {
	emptyStr := ""

	tests := []struct {
		name string
		dst  *kgateway.Image
		src  *kgateway.Image
		want *kgateway.Image
	}{
		{
			name: "non-empty digest with no tag in src clears inherited tag",
			dst: &kgateway.Image{
				Repository: new("repo"),
				Tag:        new("v1.0.0"),
			},
			src: &kgateway.Image{
				Digest: new("sha256:abc"),
			},
			want: &kgateway.Image{
				Repository: new("repo"),
				Tag:        &emptyStr,
				Digest:     new("sha256:abc"),
			},
		},
		{
			name: "non-empty tag with no digest in src clears inherited digest",
			dst: &kgateway.Image{
				Repository: new("repo"),
				Digest:     new("sha256:def"),
			},
			src: &kgateway.Image{
				Tag: new("v2.0.0"),
			},
			want: &kgateway.Image{
				Repository: new("repo"),
				Tag:        new("v2.0.0"),
				Digest:     &emptyStr,
			},
		},
		{
			name: "non-empty tag and digest in src keep both",
			dst: &kgateway.Image{
				Repository: new("repo"),
				Tag:        new("v1.0.0"),
				Digest:     new("sha256:def"),
			},
			src: &kgateway.Image{
				Tag:    new("v2.0.0"),
				Digest: new("sha256:abc"),
			},
			want: &kgateway.Image{
				Repository: new("repo"),
				Tag:        new("v2.0.0"),
				Digest:     new("sha256:abc"),
			},
		},
		{
			name: "empty-string digest in src is an explicit clear and does not clobber inherited tag",
			dst: &kgateway.Image{
				Repository: new("repo"),
				Tag:        new("v1.0.0"),
				Digest:     new("sha256:def"),
			},
			src: &kgateway.Image{
				Digest: &emptyStr,
			},
			want: &kgateway.Image{
				Repository: new("repo"),
				Tag:        new("v1.0.0"),
				Digest:     &emptyStr,
			},
		},
		{
			name: "empty-string tag in src is an explicit clear and does not clobber inherited digest",
			dst: &kgateway.Image{
				Repository: new("repo"),
				Tag:        new("v1.0.0"),
				Digest:     new("sha256:def"),
			},
			src: &kgateway.Image{
				Tag: &emptyStr,
			},
			want: &kgateway.Image{
				Repository: new("repo"),
				Tag:        &emptyStr,
				Digest:     new("sha256:def"),
			},
		},
		{
			name: "nil src returns dst unchanged",
			dst: &kgateway.Image{
				Repository: new("repo"),
				Tag:        new("v1.0.0"),
			},
			src: nil,
			want: &kgateway.Image{
				Repository: new("repo"),
				Tag:        new("v1.0.0"),
			},
		},
		{
			name: "nil dst with digest-only src yields a tag-cleared result without mutating src",
			dst:  nil,
			src: &kgateway.Image{
				Repository: new("repo"),
				Digest:     new("sha256:abc"),
			},
			want: &kgateway.Image{
				Repository: new("repo"),
				Tag:        &emptyStr,
				Digest:     new("sha256:abc"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Snapshot src so we can verify DeepMergeImage does not mutate it.
			var srcBefore *kgateway.Image
			if tt.src != nil {
				cp := *tt.src
				srcBefore = &cp
			}

			got := DeepMergeImage(tt.dst, tt.src)
			assert.Equal(t, tt.want, got)

			if tt.src != nil {
				assert.Equal(t, srcBefore, tt.src, "DeepMergeImage must not mutate src")
			}
		})
	}
}
