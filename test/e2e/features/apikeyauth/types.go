//go:build e2e

package apikeyauth

import (
	"net/http"
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	// manifests
	setupManifest                  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	apiKeyAuthManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "api-key-auth.yaml")
	apiKeyAuthManifestWithSection  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "api-key-auth-section.yaml")
	apiKeyAuthManifestQuery        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "api-key-auth-query.yaml")
	apiKeyAuthManifestCookie       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "api-key-auth-cookie.yaml")
	apiKeyAuthManifestSecretUpdate = filepath.Join(fsutils.MustGetThisDir(), "testdata", "api-key-auth-secret-update.yaml")
	apiKeyAuthManifestOverride     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "api-key-auth-override.yaml")
	apiKeyAuthManifestDisable      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "api-key-auth-disable.yaml")

	expectStatus200Success = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       nil,
	}
	expectAPIKeyAuthDenied = &matchers.HttpResponse{
		StatusCode: http.StatusUnauthorized,
		Body:       nil,
	}

	// Base test setup - common infrastructure for all tests
	setup = base.TestCase{
		Manifests: []string{setupManifest},
	}

	// Individual test cases - test-specific manifests and resources
	testCases = map[string]*base.TestCase{
		"TestAPIKeyAuthWithRouteLevelPolicy": {
			Manifests:       []string{apiKeyAuthManifestWithSection},
			MinGwApiVersion: base.GwApiRequireRouteNames,
		},
		"TestAPIKeyAuthWithHTTPRouteLevelPolicy": {
			Manifests: []string{apiKeyAuthManifest},
		},
		"TestAPIKeyAuthWithQueryParameter": {
			Manifests: []string{apiKeyAuthManifestQuery},
		},
		"TestAPIKeyAuthWithCookie": {
			Manifests: []string{apiKeyAuthManifestCookie},
		},
		"TestAPIKeyAuthWithSecretUpdate": {
			Manifests: []string{apiKeyAuthManifestSecretUpdate},
		},
		"TestAPIKeyAuthRouteOverrideGateway": {
			Manifests:       []string{apiKeyAuthManifestOverride},
			MinGwApiVersion: base.GwApiRequireRouteNames,
		},
		"TestAPIKeyAuthDisableAtRouteLevel": {
			Manifests:       []string{apiKeyAuthManifestDisable},
			MinGwApiVersion: base.GwApiRequireRouteNames,
		},
	}
)
