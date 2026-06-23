//go:build e2e

package header_modifiers

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// Manifests.
	commonManifest                                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	setupWithListenerSetsManifest                        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup-with-listenersets.yaml")
	headerModifiersRouteTrafficPolicyManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-modifiers-route.yaml")
	headerModifiersRouteListenerSetTrafficPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-modifiers-route-ls.yaml")
	headerModifiersGwTrafficPolicyManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-modifiers-gw.yaml")
	headerModifiersLsTrafficPolicyManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-modifiers-ls.yaml")
	headerModifiersFromSecretManifest                    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-modifiers-from-secret.yaml")

	// This suite reuses the shared base gateway (gateway/kgateway-base) instead of
	// deploying its own, so the only common setup is the routes and the httpbin backend.
	commonSetupManifests = []string{commonManifest, testdefaults.HttpbinManifest}

	setup = base.TestCase{
		Manifests: commonSetupManifests,
	}

	setupWithListenerSets = base.TestCase{
		Manifests: commonSetupManifests,
		ManifestsWithTransform: map[string]func(string) string{
			setupWithListenerSetsManifest: base.TransformListenerSetManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestRouteLevelHeaderModifiers": {
			Manifests: []string{headerModifiersRouteTrafficPolicyManifest},
		},
		"TestGatewayLevelHeaderModifiers": {
			Manifests: []string{headerModifiersGwTrafficPolicyManifest},
		},
		"TestMultiLevelHeaderModifiers": {
			Manifests: []string{
				headerModifiersGwTrafficPolicyManifest,
				headerModifiersRouteTrafficPolicyManifest,
			},
			ManifestsWithTransform: map[string]func(string) string{
				headerModifiersLsTrafficPolicyManifest: base.TransformListenerSetManifest,
			},
		},
		"TestMultiLevelHeaderModifiersWithListenerSet": {
			Manifests: []string{
				headerModifiersGwTrafficPolicyManifest,
				headerModifiersRouteTrafficPolicyManifest,
				headerModifiersRouteListenerSetTrafficPolicyManifest,
			},
			ManifestsWithTransform: map[string]func(string) string{
				headerModifiersLsTrafficPolicyManifest: base.TransformListenerSetManifest,
			},
			MinGwApiVersion: base.GwApiRequireListenerSets,
		},
		"TestListenerSetLevelHeaderModifiers": {
			ManifestsWithTransform: map[string]func(string) string{
				headerModifiersLsTrafficPolicyManifest: base.TransformListenerSetManifest,
			},
			MinGwApiVersion: base.GwApiRequireListenerSets,
		},
		"TestHeaderModifiersFromSecret": {
			Manifests: []string{headerModifiersFromSecretManifest},
		},
	}
)
