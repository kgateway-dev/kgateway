//go:build e2e

package tracing

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// manifests
	setupManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	otelCollectorManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "otel-collector.yaml")
	otelCollectorSecureManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "otel-collector-secure.yaml")
	policyManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tracing-policy.yaml")
	policySecureManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tracing-policy-secure.yaml")
	routeTracingPolicyManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-tracing-policy.yaml")
	routeTracingDisableManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-tracing-disable-policy.yaml")

	// collector Pods (and the Services in front of them) in the default namespace
	otelCollectorPod       = "otel-collector"
	otelCollectorSecurePod = "otel-collector-secure"

	// setup objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyService = &corev1.Service{ObjectMeta: proxyObjectMeta}

	// The plain collector is shared by every test except TestOTelTracingSecure, so it is applied
	// once here instead of per test: reapplying a bare Pod while the previous test's copy is still
	// terminating patches the dying Pod instead of creating a new one.
	setup = base.TestCase{
		Manifests: []string{e2edefaults.CurlPodManifest, setupManifest, otelCollectorManifest},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestOTelTracing": {
			Manifests: []string{policyManifest},
		},
		"TestOTelTracingSecure": {
			Manifests: []string{otelCollectorSecureManifest, policySecureManifest},
			// BackendTLSPolicy moved from experimental to standard in Gateway API 1.4, and the alpha1v3 version is not supported
			MinGwApiVersion: base.GwApiRequireBackendTLSPolicy,
		},
		"TestRouteTracingCustomAttributes": {
			Manifests: []string{policyManifest, routeTracingPolicyManifest},
		},
		"TestRouteTracingDisable": {
			Manifests: []string{policyManifest, routeTracingDisableManifest},
		},
	}
)
