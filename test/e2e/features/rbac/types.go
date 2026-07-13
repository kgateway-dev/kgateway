//go:build e2e

package rbac

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	// manifests
	setupManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	rbacManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cel-rbac.yaml")
	rbacManifestWithSections = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cel-rbac-section.yaml")
	rbacDenyManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cel-rbac-deny.yaml")
	// Core infrastructure objects that we need to track
	expectStatus200Success = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       nil,
	}
	expectRBACDenied = &matchers.HttpResponse{
		StatusCode: http.StatusForbidden,
		Body:       gomega.ContainSubstring("RBAC: access denied"),
	}

	// Base test setup - common infrastructure for all tests
	setup = base.TestCase{
		Manifests: []string{setupManifest},
	}

	// Individual test cases - test-specific manifests and resources
	testCases = map[string]*base.TestCase{
		"TestRBACHeaderAuthorizationWithRouteLevelRBAC": {
			Manifests:       []string{rbacManifestWithSections},
			MinGwApiVersion: base.GwApiRequireRouteNames,
		},
		"TestRBACHeaderAuthorization": {
			Manifests: []string{rbacManifest},
		},
		"TestRBACDenyActionAllowsNonMatchingTraffic": {
			Manifests: []string{rbacDenyManifest},
		},
	}
)
