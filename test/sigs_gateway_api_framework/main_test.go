//go:build e2e

package sigs_gateway_api_framework

import (
	"embed"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/gateway-api/conformance/utils/kubernetes"
	confsuite "sigs.k8s.io/gateway-api/conformance/utils/suite"

	"github.com/kgateway-dev/kgateway/v2/test/sigs_gateway_api_framework/common"
	"github.com/kgateway-dev/kgateway/v2/test/sigs_gateway_api_framework/features/basicrouting"
	"github.com/kgateway-dev/kgateway/v2/test/sigs_gateway_api_framework/features/header_modifiers"
)

//go:embed testdata
var sharedTestdata embed.FS

const gatewayClassName = "kgateway"

// TestE2EConformanceFramework exercises all kgateway Gateway API conformance scenarios.
// It sets up a shared gateway and backend, then runs each feature's test suite
// as parallel subtests, all using the same ConformanceTestSuite instance.
func TestE2EConformanceFramework(t *testing.T) {
	manifestFS := []fs.FS{
		sharedTestdata,
		basicrouting.ManifestFS,
		header_modifiers.ManifestFS,
	}

	suite, err := common.NewConformanceSuite(gatewayClassName, manifestFS)
	require.NoError(t, err, "failed to create conformance suite")

	// Apply shared base resources (gateway, backend) once for all features.
	common.ApplyBaseManifests(t, suite, []string{
		"testdata/gateway.yaml",
		"testdata/echo-service.yaml",
	})

	// Validate the gateway is ready before running any scenarios.
	suite.ControllerName = kubernetes.GWCMustHaveAcceptedConditionTrue(
		t, suite.Client, suite.TimeoutConfig, suite.GatewayClassName,
	)

	// Run all feature suites in parallel.
	features := []struct {
		name  string
		tests []confsuite.ConformanceTest
	}{
		{
			name:  "basicrouting",
			tests: basicrouting.Tests,
		},
		{
			name:  "header_modifiers",
			tests: header_modifiers.Tests,
		},
	}

	for _, feature := range features {
		t.Run(feature.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range feature.tests {
				t.Run(tc.ShortName, func(t *testing.T) {
					t.Parallel()
					tc.Run(t, suite)
				})
			}
		})
	}
}
