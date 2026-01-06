//go:build e2e

package apikeyauth

import (
	"net/http"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
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
	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	gatewayService = &corev1.Service{ObjectMeta: gatewayObjectMeta}

	expectStatus200Success = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       nil,
	}
	expectAPIKeyAuthDenied = &matchers.HttpResponse{
		StatusCode: http.StatusUnauthorized,
		Body:       nil,
	}

	commonSetupManifests = []string{defaults.HttpbinManifest, defaults.CurlPodManifest}
	// Base test setup - common infrastructure for all tests
	setup = base.TestCase{
		Manifests: append([]string{setupManifest}, commonSetupManifests...),
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
