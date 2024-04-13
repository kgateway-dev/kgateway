package e2e

import (
	"path/filepath"

	"github.com/solo-io/gloo/test/helpers"
)

var e2eRoot = filepath.Join(
	helpers.GlooDir(),
	"test",
	"kubernetes",
	"e2e",
)

// ManifestPath returns the absolute path to a manifest file
// This enables tests to reference files in this directory, regardless of where those tests are executed
func ManifestPath(pathPartsRelativeToE2eDir ...string) string {
	return filepath.Join(e2eRoot, filepath.Join(pathPartsRelativeToE2eDir...))
}

// FeatureManifestFile returns the absolute path to a manifest file defined in the e2e/features/manifests pacakage
// This enables tests to reference files in this directory, regardless of where those tests are executed
func FeatureManifestFile(fileName string) string {
	return ManifestPath("features", "manifests", fileName)
}
