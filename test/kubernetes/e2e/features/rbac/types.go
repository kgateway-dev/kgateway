package rbac

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// manifests
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	rbacManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cel-rbac.yaml")
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
	expectRBACDenied = &matchers.HttpResponse{
		StatusCode: http.StatusForbidden,
		Body:       gomega.ContainSubstring("RBAC: access denied"),
	}

	// Base test setup - common infrastructure for all tests
	setup = base.TestCase{
		Manifests: []string{setupManifest, defaults.HttpbinManifest, defaults.CurlPodManifest},
	}

	// Individual test cases - test-specific manifests and resources
	testCases = map[string]*base.TestCase{
		"TestRBACHeaderAuthorization": {
			Manifests: []string{rbacManifest},
		},
	}
)
