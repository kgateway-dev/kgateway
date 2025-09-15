package routereplacement

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	setupManifest                     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	strictModeInvalidPolicyManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "strict-mode-invalid-policy.yaml")
	standardModeInvalidPolicyManifest = strictModeInvalidPolicyManifest
	strictModeInvalidMatcherManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "strict-mode-invalid-matcher.yaml")
	strictModeInvalidRouteManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "strict-mode-invalid-route.yaml")

	// proxy objects (gateway deployment)
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	gatewayPort = 8080

	// route objects for testing
	invalidPolicyRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-policy-route",
			Namespace: "default",
		},
	}

	invalidMatcherRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-matcher-route",
			Namespace: "default",
		},
	}

	invalidConfigRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-config-route",
			Namespace: "default",
		},
	}
)
