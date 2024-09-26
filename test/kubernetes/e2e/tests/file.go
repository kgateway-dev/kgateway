package tests

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
)

func ManifestPath(path string) string {
	return filepath.Join(util.MustGetThisDir(), "manifests", path)
}

func ProfilePath(path string) string {
	return filepath.Join(util.MustGetThisDir(), "manifests", "profiles", path)
}

var (
	CommonRecommendationManifest = ManifestPath("common-recommendations.yaml")

	// EmptyProfilePath relies on an "empty" profile.
	// We should NOT merge with this code. The idea is to create a structure for using profiles, and then introduce them.
	EmptyProfilePath = ProfilePath("empty.yaml")

	EdgeGatewayProfilePath = ProfilePath("edge-gateway.yaml")

	KubernetesGatewayProfilePath = ProfilePath("kubernetes-gateway.yaml")

	FullGatewayProfilePath = ProfilePath("full-gateway.yaml")
)
