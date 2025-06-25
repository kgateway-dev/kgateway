package metrics

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// manifests
	setupManifest           = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	exampleRouteManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "example-route.yaml")
	metricResourcesManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "metric-resources.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw1",
		Namespace: "default",
	}

	proxyDeployment     = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService        = &corev1.Service{ObjectMeta: proxyObjectMeta}
	proxyServiceAccount = &corev1.ServiceAccount{ObjectMeta: proxyObjectMeta}

	kgatewayMetricsObjectMeta = metav1.ObjectMeta{
		Name:      "kgateway-metrics",
		Namespace: "kgateway-test",
	}

	kgatewayMetricsService = &corev1.Service{ObjectMeta: kgatewayMetricsObjectMeta}

	exampleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-svc",
			Namespace: "default",
		},
	}

	exampleRoute = &apiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-route",
			Namespace: "default",
		},
	}

	nginxPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
	}

	setup = base.SimpleTestCase{
		Manifests: []string{setupManifest, e2edefaults.CurlPodManifest},
		Resources: []client.Object{kgatewayMetricsService, exampleSvc, proxyDeployment, proxyService, proxyServiceAccount, nginxPod, e2edefaults.CurlPod},
	}

	gw2 = &apiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw2",
			Namespace: "default",
		},
	}

	exampleRoute1 = &apiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-route-1",
			Namespace: "default",
		},
	}

	exampleRoute2 = &apiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-route-2",
			Namespace: "default",
		},
	}

	exampleRouteLs1 = &apiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-route-ls1",
			Namespace: "default",
		},
	}

	listenerSet1 = &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ls1",
			Namespace: "default",
		},
	}

	testCases = map[string]*base.TestCase{
		"TestMetrics": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{exampleRouteManifest},
				Resources: []client.Object{exampleRoute},
			},
		},
		"TestResourceCountingMetrics": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{metricResourcesManifest},
				Resources: []client.Object{gw2, exampleRoute1, exampleRoute2, exampleRouteLs1, listenerSet1},
			},
		},
	}
)
