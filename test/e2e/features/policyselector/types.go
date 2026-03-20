//go:build e2e

package policyselector

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const gatewayPort = 80

var labelSelectorManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "label_selector.yaml")
