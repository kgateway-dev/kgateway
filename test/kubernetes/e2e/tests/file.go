package tests

import (
	"github.com/solo-io/skv2/codegen/util"
	"path/filepath"
)

func ManifestPath(path string) string {
	return filepath.Join(util.MustGetThisDir(), "manifests", path)
}

func ProfilePath(path string) string {
	return filepath.Join(util.MustGetThisDir(), "manifests", "profiles", path)
}

var (
	// EmptyProfilePath relies on an "empty" profile.
	// We should NOT merge with this code. The idea is to create a structure for using profiles, and then introduce them.
	EmptyProfilePath = ProfilePath("empty.yaml")

	EdgeGatewayProfilePath = ProfilePath("edge-gateway.yaml")
)
