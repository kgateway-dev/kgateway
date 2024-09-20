package tests

import (
	"github.com/solo-io/skv2/codegen/util"
	"path/filepath"
)

func ManifestPath(path string) string {
	return filepath.Join(util.MustGetThisDir(), "manifests", path)
}

func ProfilePath(path string) string {
	return filepath.Join(util.MustGetThisDir(), "manifests", "profile", path)
}

var (
	EmptyProfilePath = ProfilePath("empty.yaml")
)
