package crd_categories

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
)

var (
	emptyVsManifest = filepath.Join(util.MustGetThisDir(), "testdata/manifests", "empty-virtualservice.yaml")

	installedVs = "virtualservice.gateway.solo.io/empty-virtualservice"
)
