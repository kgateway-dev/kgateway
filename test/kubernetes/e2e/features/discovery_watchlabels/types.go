package discovery_watchlabels

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
)

var (
	serviceWithLabelsManifest           = filepath.Join(util.MustGetThisDir(), "testdata/service-with-labels.yaml")
	serviceWithModifiedLabelsManifest   = filepath.Join(util.MustGetThisDir(), "testdata/service-with-modified-labels.yaml")
	serviceWithoutLabelsManifest        = filepath.Join(util.MustGetThisDir(), "testdata/service-without-labels.yaml")
	serviceWithNoMatchingLabelsManifest = filepath.Join(util.MustGetThisDir(), "testdata/service-with-no-matching-labels.yaml")
)
