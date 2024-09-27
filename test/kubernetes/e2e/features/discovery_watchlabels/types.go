package discovery_watchlabels

import (
	"github.com/solo-io/skv2/codegen/util"
	"path/filepath"
)

var (
	serviceWithLabelsManifest         = filepath.Join(util.MustGetThisDir(), "testdata/service-with-labels.yaml")
	serviceWithModifiedLabelsManifest = filepath.Join(util.MustGetThisDir(), "testdata/service-with-modified-labels.yaml")
	serviceWithoutLabelsManifest      = filepath.Join(util.MustGetThisDir(), "testdata/service-without-labels.yaml")
)
