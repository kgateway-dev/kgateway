//go:build ignore

package crd_categories

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	emptyVsManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata/manifests", "empty-virtualservice.yaml")

	installedVs = "virtualservice.gateway.solo.io/empty-virtualservice"
)
