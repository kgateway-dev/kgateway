//go:build e2e

package e2e

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	CommonRecommendationManifest = ManifestPath("common-recommendations.yaml")

	// EmptyValuesManifestPath returns the path to a manifest with no values
	// We prefer to have our tests be explicit and require defining a values file. However, some tests
	// rely entirely on the values provided by the "profile". In those cases, the test supplies this reference
	EmptyValuesManifestPath = ManifestPath("empty-values.yaml")

	// ControlPlaneTLSManifestPath returns the path to a manifest with TLS enabled for xDS communication
	ControlPlaneTLSManifestPath = ManifestPath("controlplane-tls-helm.yaml")

	// TestAssertionsManifest returns the path to a manifest that arms test-only assertions in
	// the deployed control plane. These are instrumentation rather than recommendations, which
	// is why they are kept out of CommonRecommendationManifest; see the file for details.
	TestAssertionsManifest = ManifestPath("test-assertions.yaml")
)

// withTestAssertions appends TestAssertionsManifest to a set of Helm values files.
// It is applied by every install and upgrade the framework performs, and comes last so
// that a suite's own values cannot disable an assertion by accident.
func withTestAssertions(valuesFiles []string) []string {
	return append(append([]string{}, valuesFiles...), TestAssertionsManifest)
}

// ManifestPath returns the absolute path to a manifest file.
// These are all stored in the tests/manifests directory
func ManifestPath(pathParts ...string) string {
	manifestPathParts := append([]string{
		fsutils.MustGetThisDir(),
		"tests",
		"manifests",
	}, pathParts...)
	return filepath.Join(manifestPathParts...)
}
