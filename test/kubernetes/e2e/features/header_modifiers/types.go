package header_modifiers

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	kgatewayv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// Manifests.
	commonManifest                                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	headerModifiersRouteTrafficPolicyManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-modifiers-route.yaml")
	headerModifiersRouteListenerSetTrafficPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-modifiers-route-ls.yaml")
	headerModifiersGwTrafficPolicyManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-modifiers-gw.yaml")
	headerModifiersLsTrafficPolicyManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-modifiers-ls.yaml")

	// Resource objects.
	gateway = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
	}

	route1 = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-route-1",
			Namespace: "default",
		},
	}

	listenerSet = &gwxv1a1.XListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ls",
			Namespace: "default",
		},
	}

	route2 = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-route-2",
			Namespace: "default",
		},
	}

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	proxyDeployment     = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService        = &corev1.Service{ObjectMeta: proxyObjectMeta}
	proxyServiceAccount = &corev1.ServiceAccount{ObjectMeta: proxyObjectMeta}

	httpbinObjectMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "default",
	}

	httpbinSvc            = &corev1.Service{ObjectMeta: httpbinObjectMeta}
	httpbinDeployment     = &appsv1.Deployment{ObjectMeta: httpbinObjectMeta}
	httpbinServiceAccount = &corev1.ServiceAccount{ObjectMeta: httpbinObjectMeta}

	gwtrafficPolicy = &kgatewayv1alpha1.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "header-modifiers-gw-policy",
			Namespace: "default",
		},
	}

	routeTrafficPolicy1 = &kgatewayv1alpha1.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "header-modifiers-route-policy-1",
			Namespace: "default",
		},
	}

	routeTrafficPolicy2 = &kgatewayv1alpha1.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "header-modifiers-route-policy-2",
			Namespace: "default",
		},
	}

	lsTrafficPolicy = &kgatewayv1alpha1.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "header-modifiers-ls-policy",
			Namespace: "default",
		},
	}

	setup = base.TestCase{
		Manifests: []string{commonManifest, testdefaults.HttpbinManifest, testdefaults.CurlPodManifest},
		Resources: []client.Object{
			gateway,
			route1,
			listenerSet,
			route2,
			proxyDeployment,
			proxyService,
			proxyServiceAccount,
			httpbinSvc,
			httpbinDeployment,
			httpbinServiceAccount,
			testdefaults.CurlPod,
		},
	}

	testCases = map[string]base.TestCase{
		"TestRouteLevelHeaderModifiers": {
			Manifests: []string{headerModifiersRouteTrafficPolicyManifest},
			Resources: []client.Object{routeTrafficPolicy1},
		},
		"TestGatewayLevelHeaderModifiers": {
			Manifests: []string{headerModifiersGwTrafficPolicyManifest},
			Resources: []client.Object{gwtrafficPolicy},
		},
		"TestListenerSetLevelHeaderModifiers": {
			Manifests: []string{headerModifiersLsTrafficPolicyManifest},
			Resources: []client.Object{lsTrafficPolicy},
		},
		"TestMultiLevelHeaderModifiers": {
			Manifests: []string{
				headerModifiersGwTrafficPolicyManifest,
				headerModifiersLsTrafficPolicyManifest,
				headerModifiersRouteTrafficPolicyManifest,
				headerModifiersRouteListenerSetTrafficPolicyManifest,
			},
			Resources: []client.Object{
				gwtrafficPolicy,
				lsTrafficPolicy,
				routeTrafficPolicy1,
				routeTrafficPolicy2,
			},
		},
	}
)
