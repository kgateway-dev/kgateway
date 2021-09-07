package sidecars

import (
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
	corev1 "k8s.io/api/core/v1"
)

// GetSdsSidecar returns an SDS Sidecar of the given gloo
// release version to run alongside istio and gateway-proxy
// containers in the gateway-proxy pod
func GetSdsSidecar(version string) corev1.Container {
	return corev1.Container{
		Name:            "sds",
		Image:           "quay.io/solo-io/sds:" + version,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env: []corev1.EnvVar{
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name: statusutils.PodNamespaceEnvName,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name:  "ISTIO_MTLS_SDS_ENABLED",
				Value: "true",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "istio-certs",
				MountPath: "/etc/istio-certs/",
			},
			{
				Name:      "envoy-config",
				MountPath: "/etc/envoy",
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "sds",
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: 8234,
			},
		},
	}
}
