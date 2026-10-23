//go:build e2e

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

	bcpCookieSessionPersistenceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backendconfigpolicy-cookie-session-persistence.yaml")
	bcpHeaderSessionPersistenceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backendconfigpolicy-header-session-persistence.yaml")
	bcpDynamicRouteManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backendconfigpolicy-dynamic-route.yaml")
	bcpDynamicPolicyManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backendconfigpolicy-dynamic-policy.yaml")
	bcpOverrideRouteManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backendconfigpolicy-override-route.yaml")
	bcpOverridePolicyManifest           = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backendconfigpolicy-override-policy.yaml")
)
