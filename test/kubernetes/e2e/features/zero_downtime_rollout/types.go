//go:build ignore

package zero_downtime_rollout

import (
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	routeWithServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-with-service.yaml")
	serviceManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service-for-route.yaml")

	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	heyPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hey",
			Namespace: "hey",
		},
	}

	zeroDowntimeTestCases = map[string]*base.TestCase{
		"TestZeroDowntimeRollout": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{defaults.CurlPodManifest, serviceManifest, routeWithServiceManifest},
				Resources: []client.Object{proxyDeployment, proxyService, defaults.CurlPod, heyPod},
			},
		},
	}
)
