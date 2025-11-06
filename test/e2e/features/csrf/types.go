//go:build e2e

package csrf

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// manifests
	commonManifest                         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	csrfRouteTrafficPolicyManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "csrf-route.yaml")
	csrfGwTrafficPolicyManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "csrf-gw.yaml")
	csrfShadowedRouteTrafficPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "csrf-shadowed-route.yaml")

	// objects created by deployer after applying gateway manifest
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			commonManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestRouteLevelCSRF": {
			Manifests: []string{csrfRouteTrafficPolicyManifest},
		},
		"TestGatewayLevelCSRF": {
			Manifests: []string{csrfGwTrafficPolicyManifest},
		},
		"TestMultiLevelsCSRF": {
			Manifests: []string{csrfGwTrafficPolicyManifest, csrfRouteTrafficPolicyManifest},
		},
		"TestShadowedRouteLevelCSRF": {
			Manifests: []string{csrfShadowedRouteTrafficPolicyManifest},
		},
	}
)
