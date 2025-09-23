package tracing

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// manifests
	setupManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	otelCollectorManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "otel-collector.yaml")
	otelCollectorSecureManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "otel-collector-secure.yaml")
	policyManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tracing-policy.yaml")

	// setup objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyService = &corev1.Service{ObjectMeta: proxyObjectMeta}

	setup = base.TestCase{
		Manifests: []string{e2edefaults.CurlPodManifest, setupManifest},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestOTelTracing": {
			Manifests: []string{otelCollectorManifest, policyManifest},
		},
		"TestOTelTracingSecure": {
			Manifests: []string{otelCollectorSecureManifest, policyManifest},
		},
	}
)
