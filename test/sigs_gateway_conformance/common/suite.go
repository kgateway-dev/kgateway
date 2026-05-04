//go:build e2e

package common

import (
	"fmt"
	"io/fs"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	confconfig "sigs.k8s.io/gateway-api/conformance/utils/config"
	confsuite "sigs.k8s.io/gateway-api/conformance/utils/suite"
	"sigs.k8s.io/gateway-api/pkg/features"

	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

var suite *confsuite.ConformanceTestSuite

// SetupConformanceSuite initializes the Gateway API conformance test suite.
// It must be called once before running conformance tests.
func SetupConformanceSuite(gatewayClassName string, manifestFS []fs.FS) error {
	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return fmt.Errorf("loading kubeconfig: %w", err)
	}

	scheme := schemes.GatewayScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("registering apiextensions scheme: %w", err)
	}

	clientOpts := client.Options{Scheme: scheme}
	cl, err := client.New(cfg, clientOpts)
	if err != nil {
		return fmt.Errorf("creating controller-runtime client: %w", err)
	}
	cs, err := clientset.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("creating clientset: %w", err)
	}

	supported := confsuite.FeaturesSet{}
	supported.Insert(features.SupportGateway, features.SupportHTTPRoute)

	opts := confsuite.ConformanceOptions{
		Client:               cl,
		ClientOptions:        clientOpts,
		Clientset:            cs,
		RestConfig:           cfg,
		GatewayClassName:     gatewayClassName,
		ManifestFS:           manifestFS,
		CleanupBaseResources: true,
		SupportedFeatures:    supported,
		TimeoutConfig:        confconfig.DefaultTimeoutConfig(),
		AllowCRDsMismatch:    true,
	}

	suite, err = confsuite.NewConformanceTestSuite(opts)
	if err != nil {
		return fmt.Errorf("constructing conformance suite: %w", err)
	}

	// Custom setup: Configure Applier without invoking suite.Setup() which includes
	// TLS bootstrap and namespace constraints that aren't relevant for this POC.
	setupApplier(suite, opts.ManifestFS, gatewayClassName)

	return nil
}

// setupApplier configures the suite's Applier with our custom manifest handling.
// This replaces suite.Setup() to avoid TLS bootstrap and namespace requirements.
func setupApplier(suite *confsuite.ConformanceTestSuite, manifestFS []fs.FS, gatewayClassName string) {
	// The conformance Applier rewrites every Gateway's spec.gatewayClassName to
	// Applier.GatewayClass during manifest application. Configure these settings
	// instead of relying on suite.Setup() which includes unneeded TLS bootstrap.
	suite.Applier.ManifestFS = manifestFS
	suite.Applier.GatewayClass = gatewayClassName
}

// GetSuite returns the initialized conformance test suite.
func GetSuite() *confsuite.ConformanceTestSuite {
	return suite
}

// ApplyBaseManifests applies base manifests (gateway, backend) with auto-cleanup.
// The manifests are applied at suite level with t.Cleanup registration for automatic
// resource deletion. Each call applies its manifests independently.
func ApplyBaseManifests(t *testing.T, manifests []string) {
	t.Helper()
	if suite == nil {
		t.Fatalf("conformance suite not initialized; call SetupConformanceSuite first")
	}
	for _, manifest := range manifests {
		suite.Applier.MustApplyWithCleanup(t, suite.Client, suite.TimeoutConfig, manifest, true)
	}
}
