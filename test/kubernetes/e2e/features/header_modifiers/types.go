package header_modifiers

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	setup = base.TestCase{
		Manifests: []string{commonManifest, testdefaults.HttpbinManifest, testdefaults.CurlPodManifest},
	}

	testCases = map[string]*base.TestCase{
		"TestRouteLevelHeaderModifiers": {
			Manifests: []string{headerModifiersRouteTrafficPolicyManifest},
		},
		"TestGatewayLevelHeaderModifiers": {
			Manifests: []string{headerModifiersGwTrafficPolicyManifest},
		},
		"TestListenerSetLevelHeaderModifiers": {
			Manifests: []string{headerModifiersLsTrafficPolicyManifest},
		},
		"TestMultiLevelHeaderModifiers": {
			Manifests: []string{
				headerModifiersGwTrafficPolicyManifest,
				headerModifiersLsTrafficPolicyManifest,
				headerModifiersRouteTrafficPolicyManifest,
				headerModifiersRouteListenerSetTrafficPolicyManifest,
			},
		},
	}
)
