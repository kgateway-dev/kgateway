package path_matching

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// manifests
	setupManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	exactManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "exact.yaml")
	prefixManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "prefix.yaml")
	regexManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "regex.yaml")
	prefixRewriteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "prefix-rewrite.yaml")

	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	gatewayService = &corev1.Service{ObjectMeta: gatewayObjectMeta}

	setup = base.TestCase{
		Manifests: []string{e2edefaults.CurlPodManifest, setupManifest},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestExactMatch": {
			Manifests: []string{exactManifest},
		},
		"TestPrefixMatch": {
			Manifests: []string{prefixManifest},
		},
		"TestRegexMatch": {
			Manifests: []string{regexManifest},
		},
		"TestPrefixRewrite": {
			Manifests: []string{prefixRewriteManifest},
		},
	}
)
