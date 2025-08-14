package header_modifiers

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	kgatewayv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
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

	httpbinSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "default",
		},
	}

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
)
