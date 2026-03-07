//go:build e2e

package csrf

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// manifests
	commonManifest                         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	csrfRouteTrafficPolicyManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "csrf-route.yaml")
	csrfGwTrafficPolicyManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "csrf-gw.yaml")
	csrfShadowedRouteTrafficPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "csrf-shadowed-route.yaml")

	setup = base.TestCase{
		Manifests: []string{
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
