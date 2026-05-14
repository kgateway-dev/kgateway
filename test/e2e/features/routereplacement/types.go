//go:build e2e

package routereplacement

import (
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

const defaultNamespace = "route-replacement-test"

func installNamespace() string {
	ns, _ := envutils.LookupOrDefault(testutils.InstallNamespace, "route-replacement-test")
	return ns
}

func transformInstallNamespace(content string) string {
	return strings.ReplaceAll(content, defaultNamespace, installNamespace())
}

var (
	setupManifest                         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	routeAttachedInvalidPolicyManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-attached-invalid-policy.yaml")
	invalidMatcherManifest                = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-matcher.yaml")
	invalidRouteRuleFilterManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-route-rule-filter.yaml")
	gatewayWideInvalidPolicyManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-wide-invalid-policy.yaml")
	listenerSpecificInvalidPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-specific-invalid-policy.yaml")
	listenerMergeBlastRadiusManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-merge-blast-radius.yaml")
	missingExtensionManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-attached-missing-extension.yaml")
	dualErrorManifest                     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-attached-dual-error.yaml")

	// Service that fronts the kgateway controller's /metrics endpoint, created
	// by setup.yaml in the install namespace (where the controller pod lives).
	kgatewayMetricsObjectMeta = metav1.ObjectMeta{
		Name:      "kgateway-metrics",
		Namespace: installNamespace(),
	}

	invalidPolicy = metav1.ObjectMeta{
		Name:      "invalid-traffic-policy",
		Namespace: "default",
	}

	missingExtensionRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-extension-route",
			Namespace: "default",
		},
	}
	missingExtensionPolicy = metav1.ObjectMeta{
		Name:      "missing-extension-policy",
		Namespace: "default",
	}

	dualErrorRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dual-error-route",
			Namespace: "default",
		},
	}
	dualErrorInvalidConfigPolicy = metav1.ObjectMeta{
		Name:      "dual-error-invalid-config-policy",
		Namespace: "default",
	}

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
