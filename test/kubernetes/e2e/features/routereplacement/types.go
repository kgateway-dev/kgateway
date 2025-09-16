package routereplacement

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	setupManifest                         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	strictModeInvalidPolicyManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "strict-mode-invalid-policy.yaml")
	standardModeInvalidPolicyManifest     = strictModeInvalidPolicyManifest
	strictModeInvalidMatcherManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "strict-mode-invalid-matcher.yaml")
	strictModeInvalidRouteManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "strict-mode-invalid-route.yaml")
	gatewayWideInvalidPolicyManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-wide-invalid-policy.yaml")
	listenerSpecificInvalidPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-specific-invalid-policy.yaml")
	listenerMergeBlastRadiusManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-merge-blast-radius.yaml")

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	gatewayWideProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw-gateway-wide",
		Namespace: "default",
	}

	listenerSpecificProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw-listener-specific",
		Namespace: "default",
	}

	listenerIsolationProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw-listener-isolation",
		Namespace: "default",
	}

	gatewayPort = 8080

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

	gatewayWideRoute8080 = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-8080",
			Namespace: "default",
		},
	}
	gatewayWideRoute8081 = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-8081",
			Namespace: "default",
		},
	}
	listenerAffectedRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-affected",
			Namespace: "default",
		},
	}
	listenerUnaffectedRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-unaffected",
			Namespace: "default",
		},
	}

	mergeAffectedRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-affected",
			Namespace: "default",
		},
	}
	mergeUnaffectedRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-collateral",
			Namespace: "default",
		},
	}
	mergeIsolatedRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-isolated",
			Namespace: "default",
		},
	}
)
