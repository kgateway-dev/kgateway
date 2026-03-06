//go:build e2e

package extauth

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
)

var (
	// common resources
	simpleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-svc",
			Namespace: "kgateway-base",
		},
	}
	simpleDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend-0",
			Namespace: "kgateway-base",
		},
	}

	proxyObjMeta = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: "kgateway-base",
	}

	// ExtAuth service and extension
	extAuthSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ext-authz",
			Namespace: "kgateway-base",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 8000,
				},
			},
			Selector: map[string]string{
				defaults.WellKnownAppLabel: "extauth",
			},
		},
	}

	extAuthExtension = &kgateway.GatewayExtension{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic-extauth",
			Namespace: "kgateway-base",
		},
		Spec: kgateway.GatewayExtensionSpec{
			ExtAuth: &kgateway.ExtAuthProvider{
				GrpcService: &kgateway.ExtGrpcService{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: "ext-authz",
						},
					},
				},
			},
		},
	}

	// MARK per test data
	basicSecureRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hey-its-a-route",
			Namespace: "kgateway-base",
		},
	}
	gatewayAttachedTrafficPolicy = &kgateway.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-policy",
			Namespace: "kgateway-base",
		},
	}
	insecureRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-example-insecure",
			Namespace: "kgateway-base",
		},
	}
	insecureTrafficPolicy = &kgateway.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "insecure-route-policy",
			Namespace: "kgateway-base",
		},
	}
	secureRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-example-secure",
			Namespace: "kgateway-base",
		},
	}
	secureTrafficPolicy = &kgateway.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secure-route-policy",
			Namespace: "kgateway-base",
		},
	}

	// Manifest files
	gatewayWithRouteManifest     = getTestFile("common.yaml")
	simpleServiceManifest        = getTestFile("service.yaml")
	extAuthManifest              = getTestFile("ext-authz-server.yaml")
	securedGatewayPolicyManifest = getTestFile("secured-gateway-policy.yaml")
	securedRouteManifest         = getTestFile("secured-route.yaml")
	insecureRouteManifest        = getTestFile("insecure-route.yaml")
)

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
