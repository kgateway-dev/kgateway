package dfp

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// common resources
	simpleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-svc",
			Namespace: "kgateway-test",
		},
	}
	simpleDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend-0",
			Namespace: "kgateway-test",
		},
	}

	proxyObjMeta = metav1.ObjectMeta{
		Name:      "super-gateway",
		Namespace: "kgateway-test",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyObjMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjMeta}

	// MARK per test data
	dfpRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-dfp",
			Namespace: "kgateway-test",
		},
	}
	dfpBackend = &v1alpha1.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dfp-backend",
			Namespace: "kgateway-test",
		},
	}

	// Manifest files
	gatewayWithRouteManifest = getTestFile("common.yaml")
	simpleServiceManifest    = getTestFile("service.yaml")
)

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
