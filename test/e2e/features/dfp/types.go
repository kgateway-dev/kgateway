//go:build e2e

package dfp

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

	// MARK per test data
	dfpRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-dfp",
			Namespace: "kgateway-base",
		},
	}
	dfpBackend = &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dfp-backend",
			Namespace: "kgateway-base",
		},
	}

	// Manifest files
	gatewayWithRouteManifest = getTestFile("common.yaml")
	simpleServiceManifest    = getTestFile("service.yaml")
)

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
