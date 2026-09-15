package deployer

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

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
							Replicas: new(int32(5)),
						},
					},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: new(int32(5)),
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
							Replicas: new(int32(2)),
						},
					},
				},
			},
			src: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: new(int32(3)),
						},
					},
				},
			},
			want: &kgateway.GatewayParameters{
				Spec: kgateway.GatewayParametersSpec{
					Kube: &kgateway.KubernetesProxyConfig{
						Deployment: &kgateway.ProxyDeployment{
							Replicas: new(int32(3)),
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
							Replicas: new(int32(2)),
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
							Replicas: new(int32(2)),
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
							Type: new(corev1.ServiceTypeLoadBalancer),
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
							Type:              new(corev1.ServiceTypeLoadBalancer),
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
							Type:              new(corev1.ServiceTypeLoadBalancer),
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
							Type:              new(corev1.ServiceTypeLoadBalancer),
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
							Type:              new(corev1.ServiceTypeLoadBalancer),
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
							Type:              new(corev1.ServiceTypeLoadBalancer),
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

func TestDeepMergeSecurityContextWindowsOptions(t *testing.T) {
	const gmsaSpec = `{"apiVersion":"windows.k8s.io/v1","kind":"GMSACredentialSpec"}`

	tests := []struct {
		name string
		dst  *corev1.WindowsSecurityContextOptions
		src  *corev1.WindowsSecurityContextOptions
		want *corev1.WindowsSecurityContextOptions
	}{
		{
			name: "src gmsaCredentialSpecName overrides dst",
			dst: &corev1.WindowsSecurityContextOptions{
				GMSACredentialSpecName: new("default-gmsa"),
				RunAsUserName:          new("default-user"),
			},
			src: &corev1.WindowsSecurityContextOptions{
				GMSACredentialSpecName: new("override-gmsa"),
			},
			want: &corev1.WindowsSecurityContextOptions{
				GMSACredentialSpecName: new("override-gmsa"),
				RunAsUserName:          new("default-user"),
			},
		},
		{
			name: "dst gmsaCredentialSpecName is kept when src does not set it",
			dst: &corev1.WindowsSecurityContextOptions{
				GMSACredentialSpecName: new("default-gmsa"),
			},
			src: &corev1.WindowsSecurityContextOptions{
				RunAsUserName: new("override-user"),
			},
			want: &corev1.WindowsSecurityContextOptions{
				GMSACredentialSpecName: new("default-gmsa"),
				RunAsUserName:          new("override-user"),
			},
		},
		{
			name: "gmsaCredentialSpec does not leak into gmsaCredentialSpecName",
			dst: &corev1.WindowsSecurityContextOptions{
				GMSACredentialSpec: new(gmsaSpec),
			},
			src: &corev1.WindowsSecurityContextOptions{
				GMSACredentialSpecName: new("override-gmsa"),
			},
			want: &corev1.WindowsSecurityContextOptions{
				GMSACredentialSpecName: new("override-gmsa"),
				GMSACredentialSpec:     new(gmsaSpec),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeepMergeSecurityContext(
				&corev1.SecurityContext{WindowsOptions: tt.dst.DeepCopy()},
				&corev1.SecurityContext{WindowsOptions: tt.src.DeepCopy()},
			)
			assert.Equal(t, tt.want, got.WindowsOptions, "container securityContext.windowsOptions")

			gotPod := deepMergePodSecurityContext(
				&corev1.PodSecurityContext{WindowsOptions: tt.dst.DeepCopy()},
				&corev1.PodSecurityContext{WindowsOptions: tt.src.DeepCopy()},
			)
			assert.Equal(t, tt.want, gotPod.WindowsOptions, "pod securityContext.windowsOptions")
		})
	}
}

func assertAllFieldsSet(t *testing.T, v any) {
	t.Helper()
	rv := reflect.ValueOf(v).Elem()
	for i := range rv.NumField() {
		assert.False(t, rv.Field(i).IsZero(), "fixture must populate %s.%s", rv.Type().Name(), rv.Type().Field(i).Name)
	}
}

func TestDeepMergeSecurityContextAllFields(t *testing.T) {
	dst := &corev1.SecurityContext{
		Capabilities:             &corev1.Capabilities{Add: []corev1.Capability{"NET_BIND_SERVICE"}, Drop: []corev1.Capability{"KILL"}},
		Privileged:               new(true),
		SELinuxOptions:           &corev1.SELinuxOptions{User: "system_u", Role: "system_r", Type: "container_t", Level: "s0"},
		WindowsOptions:           &corev1.WindowsSecurityContextOptions{RunAsUserName: new("default-user")},
		RunAsUser:                new(int64(1000)),
		RunAsGroup:               new(int64(1000)),
		RunAsNonRoot:             new(false),
		ReadOnlyRootFilesystem:   new(false),
		AllowPrivilegeEscalation: new(true),
		ProcMount:                new(corev1.DefaultProcMount),
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined},
		AppArmorProfile:          &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeRuntimeDefault},
	}
	src := &corev1.SecurityContext{
		Capabilities:             &corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN"}, Drop: []corev1.Capability{"ALL"}},
		Privileged:               new(false),
		SELinuxOptions:           &corev1.SELinuxOptions{User: "user_u", Role: "user_r", Type: "spc_t", Level: "s0:c1"},
		WindowsOptions:           &corev1.WindowsSecurityContextOptions{RunAsUserName: new("override-user")},
		RunAsUser:                new(int64(1001)),
		RunAsGroup:               new(int64(1002)),
		RunAsNonRoot:             new(true),
		ReadOnlyRootFilesystem:   new(true),
		AllowPrivilegeEscalation: new(false),
		ProcMount:                new(corev1.UnmaskedProcMount),
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost, LocalhostProfile: new("seccomp.json")},
		AppArmorProfile:          &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeLocalhost, LocalhostProfile: new("k8s-apparmor")},
	}
	assertAllFieldsSet(t, dst)
	assertAllFieldsSet(t, src)

	t.Run("src fields are copied into empty dst", func(t *testing.T) {
		got := DeepMergeSecurityContext(&corev1.SecurityContext{}, src.DeepCopy())
		assert.Equal(t, src, got)
	})

	t.Run("dst fields are kept when src is empty", func(t *testing.T) {
		got := DeepMergeSecurityContext(dst.DeepCopy(), &corev1.SecurityContext{})
		assert.Equal(t, dst, got)
	})

	t.Run("src fields override populated dst and lists are appended", func(t *testing.T) {
		want := src.DeepCopy()
		want.Capabilities.Add = []corev1.Capability{"NET_BIND_SERVICE", "NET_ADMIN"}
		want.Capabilities.Drop = []corev1.Capability{"KILL", "ALL"}

		got := DeepMergeSecurityContext(dst.DeepCopy(), src.DeepCopy())
		assert.Equal(t, want, got)
	})
}

func TestDeepMergePodSecurityContextAllFields(t *testing.T) {
	dst := &corev1.PodSecurityContext{
		SELinuxOptions:           &corev1.SELinuxOptions{User: "system_u", Role: "system_r", Type: "container_t", Level: "s0"},
		WindowsOptions:           &corev1.WindowsSecurityContextOptions{RunAsUserName: new("default-user")},
		RunAsUser:                new(int64(1000)),
		RunAsGroup:               new(int64(1000)),
		RunAsNonRoot:             new(false),
		SupplementalGroups:       []int64{2000},
		SupplementalGroupsPolicy: new(corev1.SupplementalGroupsPolicyMerge),
		FSGroup:                  new(int64(1000)),
		Sysctls:                  []corev1.Sysctl{{Name: "net.core.somaxconn", Value: "1024"}},
		FSGroupChangePolicy:      new(corev1.FSGroupChangeAlways),
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined},
		AppArmorProfile:          &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeRuntimeDefault},
		SELinuxChangePolicy:      new(corev1.SELinuxChangePolicyMountOption),
	}
	src := &corev1.PodSecurityContext{
		SELinuxOptions:           &corev1.SELinuxOptions{User: "user_u", Role: "user_r", Type: "spc_t", Level: "s0:c1"},
		WindowsOptions:           &corev1.WindowsSecurityContextOptions{RunAsUserName: new("override-user")},
		RunAsUser:                new(int64(1001)),
		RunAsGroup:               new(int64(1002)),
		RunAsNonRoot:             new(true),
		SupplementalGroups:       []int64{3000},
		SupplementalGroupsPolicy: new(corev1.SupplementalGroupsPolicyStrict),
		FSGroup:                  new(int64(1003)),
		Sysctls:                  []corev1.Sysctl{{Name: "net.ipv4.ip_unprivileged_port_start", Value: "0"}},
		FSGroupChangePolicy:      new(corev1.FSGroupChangeOnRootMismatch),
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost, LocalhostProfile: new("seccomp.json")},
		AppArmorProfile:          &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeLocalhost, LocalhostProfile: new("k8s-apparmor")},
		SELinuxChangePolicy:      new(corev1.SELinuxChangePolicyRecursive),
	}
	assertAllFieldsSet(t, dst)
	assertAllFieldsSet(t, src)

	t.Run("src fields are copied into empty dst", func(t *testing.T) {
		got := deepMergePodSecurityContext(&corev1.PodSecurityContext{}, src.DeepCopy())
		assert.Equal(t, src, got)
	})

	t.Run("dst fields are kept when src is empty", func(t *testing.T) {
		got := deepMergePodSecurityContext(dst.DeepCopy(), &corev1.PodSecurityContext{})
		assert.Equal(t, dst, got)
	})

	t.Run("src fields override populated dst and lists are appended", func(t *testing.T) {
		want := src.DeepCopy()
		want.SupplementalGroups = []int64{2000, 3000}
		want.Sysctls = []corev1.Sysctl{
			{Name: "net.core.somaxconn", Value: "1024"},
			{Name: "net.ipv4.ip_unprivileged_port_start", Value: "0"},
		}

		got := deepMergePodSecurityContext(dst.DeepCopy(), src.DeepCopy())
		assert.Equal(t, want, got)
	})
}

func TestDeepMergeSecurityContextListSemantics(t *testing.T) {
	tests := []struct {
		name string
		dst  []int64
		src  []int64
		want []int64
	}{
		{name: "nil src inherits dst", dst: []int64{1}, src: nil, want: []int64{1}},
		{name: "empty src clears dst", dst: []int64{1}, src: []int64{}, want: []int64{}},
		{name: "non-empty src is appended to dst", dst: []int64{1}, src: []int64{2}, want: []int64{1, 2}},
		{name: "nil dst takes src", dst: nil, src: []int64{2}, want: []int64{2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPod := deepMergePodSecurityContext(
				&corev1.PodSecurityContext{
					SupplementalGroups: tt.dst,
					Sysctls:            toSysctls(tt.dst),
				},
				&corev1.PodSecurityContext{
					SupplementalGroups: tt.src,
					Sysctls:            toSysctls(tt.src),
				},
			)
			assert.Equal(t, tt.want, gotPod.SupplementalGroups, "pod securityContext.supplementalGroups")
			assert.Equal(t, toSysctls(tt.want), gotPod.Sysctls, "pod securityContext.sysctls")

			got := DeepMergeSecurityContext(
				&corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: toCapabilities(tt.dst), Drop: toCapabilities(tt.dst)}},
				&corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: toCapabilities(tt.src), Drop: toCapabilities(tt.src)}},
			)
			assert.Equal(t, toCapabilities(tt.want), got.Capabilities.Add, "container securityContext.capabilities.add")
			assert.Equal(t, toCapabilities(tt.want), got.Capabilities.Drop, "container securityContext.capabilities.drop")
		})
	}
}

func toSysctls(ids []int64) []corev1.Sysctl {
	if ids == nil {
		return nil
	}
	out := make([]corev1.Sysctl, 0, len(ids))
	for _, id := range ids {
		out = append(out, corev1.Sysctl{Name: fmt.Sprintf("sysctl.%d", id), Value: "1"})
	}
	return out
}

func toCapabilities(ids []int64) []corev1.Capability {
	if ids == nil {
		return nil
	}
	out := make([]corev1.Capability, 0, len(ids))
	for _, id := range ids {
		out = append(out, corev1.Capability(fmt.Sprintf("CAP_%d", id)))
	}
	return out
}
