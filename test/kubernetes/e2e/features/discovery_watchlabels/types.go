//go:build ignore

package discovery_watchlabels

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	serviceWithLabelsManifest           = filepath.Join(fsutils.MustGetThisDir(), "testdata/service-with-labels.yaml")
	serviceWithModifiedLabelsManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata/service-with-modified-labels.yaml")
	serviceWithoutLabelsManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata/service-without-labels.yaml")
	serviceWithNoMatchingLabelsManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata/service-with-no-matching-labels.yaml")
)
