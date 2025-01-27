package zero_downtime_rollout

import (
	"path/filepath"

	"github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e/tests/base"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/solo-io/skv2/codegen/util"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	routeWithServiceManifest = filepath.Join(util.MustGetThisDir(), "testdata", "route-with-service.yaml")
	serviceManifest          = filepath.Join(util.MustGetThisDir(), "testdata", "service-for-route.yaml")

	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
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
