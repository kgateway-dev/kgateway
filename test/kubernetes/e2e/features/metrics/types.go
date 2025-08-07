package metrics

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// manifests
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

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

	nginxPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
	}

	gw1 = &apiv1.Gateway{
		ObjectMeta: proxyObjectMeta,
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

	httpListenerPolicy1 = &v1alpha1.HTTPListenerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-listener-policy-all-fields",
			Namespace: "default",
		},
	}

	setup = base.TestCase{
		Manifests: []string{setupManifest, e2edefaults.CurlPodManifest},
		Resources: []client.Object{
			kgatewayMetricsService,
			exampleSvc,
			proxyDeployment,
			proxyService,
			proxyServiceAccount,
			gw1,
			gw2,
			exampleRoute1,
			exampleRoute2,
			exampleRouteLs1,
			listenerSet1,
			httpListenerPolicy1,
			e2edefaults.CurlPod,
		},
	}

	testCases = map[string]base.TestCase{
		"TestMetrics": base.TestCase{},
	}
)
