//go:build e2e

package path_matching

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// manifests
	setupManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	exactManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "exact.yaml")
	prefixManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "prefix.yaml")
	regexManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "regex.yaml")
	prefixRewriteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "prefix-rewrite.yaml")

	setup = base.TestCase{
		Manifests: []string{setupManifest},
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
