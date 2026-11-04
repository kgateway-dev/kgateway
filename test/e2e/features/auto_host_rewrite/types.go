//go:build e2e

package auto_host_rewrite

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var autoHostRewriteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "auto_host_rewrite.yaml")
