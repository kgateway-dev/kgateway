package sidecars

import (
	"fmt"

	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Sidecar for Istio 1.7.x releases, also works for Istio 1.8.x, 1.9.x and 1.10.x releases
func generateIstio17to119Sidecar(version, jwtPolicy string, istioMetaMeshID string, istioMetaClusterID string, istioDiscoveryAddress string) *corev1.Container {
	sidecar := &corev1.Container{
		Name:  "istio-proxy",
		Image: "docker.io/istio/proxyv2:" + version,
		Args: []string{
			"proxy",
			"sidecar",
			"--domain",
			"$(POD_NAMESPACE).svc.cluster.local",
			"--configPath",
			"/etc/istio/proxy",
			"--binaryPath",
			"/usr/local/bin/envoy",
			"--serviceCluster",
			"istio-proxy-prometheus",
			"--drainDuration",
			"45s",
			"--parentShutdownDuration",
			"1m0s",
			"--proxyLogLevel=warning",
			"--proxyComponentLogLevel=misc:error",
			"--connectTimeout",
			"10s",
			"--proxyAdminPort",
			"15000",
			"--controlPlaneAuthPolicy",
			"NONE",
			"--dnsRefreshRate",
			"300s",
			"--statusPort",
			"15021",
			"--trust-domain=cluster.local",
			"--controlPlaneBootstrap=false",
		},
		Env: []corev1.EnvVar{
			{
				Name:  "OUTPUT_CERTS",
				Value: "/etc/istio-certs",
			},
			{
				Name:  "JWT_POLICY",
				Value: jwtPolicy,
			},
			{
				Name:  "PILOT_CERT_PROVIDER",
				Value: "istiod",
			},
			{
				Name:  "CA_ADDR",
				Value: istioDiscoveryAddress,
			},
			{
				Name:  "ISTIO_META_MESH_ID",
				Value: istioMetaMeshID,
			},
			{
				Name:  "ISTIO_META_CLUSTER_ID",
				Value: istioMetaClusterID,
			},
			{
				Name:  "PROXY_CONFIG",
				Value: fmt.Sprintf("{ \"discoveryAddress\": %s }", istioDiscoveryAddress),
			},
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
				Name: "INSTANCE_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
			{
				Name: "SERVICE_ACCOUNT",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "spec.serviceAccountName",
					},
				},
			},
			{
				Name: "HOST_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.hostIP",
					},
				},
			},
			{
				Name: "ISTIO_META_POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
				},
			},
			{
				Name: "ISTIO_META_CONFIG_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.namespace",
					},
				},
			},
		},
		ImagePullPolicy: corev1.PullIfNotPresent,
		ReadinessProbe: &corev1.Probe{
			FailureThreshold: 30,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/healthz/ready",
					Port:   intstr.FromInt(15021),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 1,
			PeriodSeconds:       2,
			SuccessThreshold:    1,
			TimeoutSeconds:      1,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "istiod-ca-cert",
				MountPath: "/var/run/secrets/istio",
			},
			{
				Name:      "istio-envoy",
				MountPath: "/etc/istio/proxy",
			},
			{
				Name:      "istio-certs",
				MountPath: "/etc/istio-certs/",
			},
		},
	}
	// For third-party-jwt, use istio-token
	if jwtPolicy == "third-party-jwt" {
		istioToken := corev1.VolumeMount{
			Name:      "istio-token",
			MountPath: "/var/run/secrets/tokens",
		}
		sidecar.VolumeMounts = append(sidecar.VolumeMounts, istioToken)
	}

	return sidecar
}
