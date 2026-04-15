//go:build e2e

package tls

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var routeManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route.yaml")
