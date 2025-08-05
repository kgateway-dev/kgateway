package routereplacement

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	setupManifest                     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	strictModeInvalidPolicyManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "strict-mode-invalid-policy.yaml")
	standardModeInvalidPolicyManifest = strictModeInvalidPolicyManifest

	// proxy objects (gateway deployment)
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment     = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService        = &corev1.Service{ObjectMeta: proxyObjectMeta}
	proxyServiceAccount = &corev1.ServiceAccount{ObjectMeta: proxyObjectMeta}

	gatewayPort = 8080

	// gateway object
	gateway = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
	}

	// route objects for testing
	invalidPolicyRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-policy-route",
			Namespace: "default",
		},
	}
)
