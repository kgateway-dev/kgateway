//go:build e2e

package apikeyauth

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	apiKeyAuthManifestDuplicate    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "api-key-auth-duplicate.yaml")

	expectStatus200Success = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       nil,
	}
	expectAPIKeyAuthDenied = &matchers.HttpResponse{
		StatusCode: http.StatusUnauthorized,
		Body:       nil,
	}
	// proxyObjectMeta targets the shared gateway deployment for Envoy admin API access.
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: "kgateway-base",
	}

	// expectRouteReplaced matches the direct response kgateway substitutes for a route whose
	// policy failed to translate. The body is what makes the assertion specific: a bare 500 could
	// come from the upstream, from Envoy itself, or from a connection failure, none of which would
	// show that the route was replaced rather than left serving stale config.
	expectRouteReplaced = &matchers.HttpResponse{
		StatusCode: http.StatusInternalServerError,
		Body:       gomega.ContainSubstring("invalid route configuration detected and replaced with a direct response."),
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
		"TestAPIKeyAuthDuplicateKeyValues": {
			Manifests: []string{apiKeyAuthManifestDuplicate},
		},
		"TestAPIKeyAuthDisableAtRouteLevel": {
			Manifests:       []string{apiKeyAuthManifestDisable},
			MinGwApiVersion: base.GwApiRequireRouteNames,
		},
	}
)
