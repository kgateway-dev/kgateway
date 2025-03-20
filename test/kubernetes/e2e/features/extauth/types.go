package extauth

import (
	"os"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func ptr[T any](v T) *T {
	return &v
}

var (
	// Common objects used across tests
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "example-gateway",
		Namespace: "default",
	}

	// Service and deployment for the echo service
	simpleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 8080,
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name": "echo",
			},
		},
	}

	// Proxy service and deployment
	proxyService = &corev1.Service{
		ObjectMeta: proxyObjectMeta,
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 8080,
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name": "gw",
			},
		},
	}

	proxyServiceAccount = &corev1.ServiceAccount{
		ObjectMeta: proxyObjectMeta,
	}

	proxyDeployment = &appsv1.Deployment{
		ObjectMeta: proxyObjectMeta,
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "gw",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": "gw",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: proxyObjectMeta.Name,
					Containers: []corev1.Container{
						{
							Name:  "proxy",
							Image: "envoyproxy/envoy:v1.28-latest",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	}

	// ExtAuth service and extension
	extAuthSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "extauth",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 8080,
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name": "extauth",
			},
		},
	}

	extAuthExtension = &v1alpha1.GatewayExtension{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-auth-extension",
			Namespace: "default",
		},
		Spec: v1alpha1.GatewayExtensionSpec{
			Type: v1alpha1.GatewayExtensionTypeExtAuth,
			ExtAuth: &v1alpha1.ExtAuthProvider{
				BackendRef: &gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: "extauth",
						Port: ptr(gwv1.PortNumber(8080)),
					},
				},
			},
		},
	}

	// Manifest files
	simpleServiceManifest         = readTestData("service.yaml")
	gatewayWithRouteManifest      = readTestData("common.yaml")
	extAuthServiceManifest        = readTestData("ext-authz-server.yaml")
	extAuthExtensionManifest      = readTestData("securing-at-route.yaml")
	routePolicyWithExtAuthEnabled = readTestData("route-policy-extauth-enabled.yaml")
)

func readTestData(filename string) string {
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		panic(err)
	}
	return string(data)
}
