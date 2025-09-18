package session_persistence

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// Manifest paths
	cookieSessionPersistenceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cookie-session-persistence.yaml")
	headerSessionPersistenceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-session-persistence.yaml")
	echoServiceManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "echo-service.yaml")
)
