package e2e

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
)

var (
	EdgeGatewayProfilePath = ProfilePath("edge-gateway.yaml")

	KubernetesGatewayProfilePath = ProfilePath("kubernetes-gateway.yaml")

	FullGatewayProfilePath = ProfilePath("full-gateway.yaml")

	CommonRecommendationManifest = ManifestPath("common-recommendations.yaml")

	// EmptyValuesManifestPath returns the path to a manifest with no values
	// We prefer to have our tests be explicit and require defining a values file. However, some tests
	// rely entirely on the values provided by the "profile". In those cases, the test supplies this reference
	EmptyValuesManifestPath = ManifestPath("empty-values.yaml")
)

// ManifestPath returns the absolute path to a manifest file.
// These are all stored in the tests/manifests directory
func ManifestPath(pathParts ...string) string {
	manifestPathParts := append([]string{
		util.MustGetThisDir(),
		"tests",
		"manifests",
	}, pathParts...)
	return filepath.Join(manifestPathParts...)
}

// ProfilePath returns the absolute path to a profile file.
// These are all stored in the tests/manifests/profiles directory
func ProfilePath(path string) string {
	return ManifestPath("profiles", path)
}
