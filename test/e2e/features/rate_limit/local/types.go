//go:build e2e

package local_rate_limit

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	simpleServiceManifest = getTestFile("service.yaml")
	// local rate limit traffic policies
	routeLocalRateLimitManifest         = getTestFile("route-local-rate-limit.yaml")
	gwLocalRateLimitManifest            = getTestFile("gw-local-rate-limit.yaml")
	disabledRouteLocalRateLimitManifest = getTestFile("route-local-rate-limit-disabled.yaml")
	httpRoutesManifest                  = getTestFile("httproutes.yaml")
	extensionRefManifest                = getTestFile("extensionref-rl.yaml")

	// objects from gateway manifest
	route = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-route",
			Namespace: "kgateway-base",
		},
	}
	route2 = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-route-2",
			Namespace: "kgateway-base",
		},
	}

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

	routeRateLimitTrafficPolicy = &kgateway.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-rl-policy",
			Namespace: "kgateway-base",
		},
	}

	gwRateLimitTrafficPolicy = &kgateway.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-rl-policy",
			Namespace: "kgateway-base",
		},
	}
)

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
