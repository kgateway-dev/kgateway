package validation_strict

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
)

var (
	invalidUpstream = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-upstream.yaml")
)
