package validation_allow_warnings

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
)

const (
	secretName       = "tls-secret"
	unusedSecretName = "tls-secret-unused"
)

var (
	vs       = filepath.Join(util.MustGetThisDir(), "testdata", "vs.yaml")
	upstream = filepath.Join(util.MustGetThisDir(), "testdata", "upstream.yaml")
)
