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

// NewConformanceSuite creates and returns a Gateway API conformance test suite.
// The caller owns the returned suite and must pass it explicitly to helpers like
// ApplyBaseManifests and ConformanceTest.Run.
func NewConformanceSuite(gatewayClassName string, manifestFS []fs.FS) (*confsuite.ConformanceTestSuite, error) {
	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	scheme := schemes.GatewayScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("registering apiextensions scheme: %w", err)
	}

	clientOpts := client.Options{Scheme: scheme}
	cl, err := client.New(cfg, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("creating controller-runtime client: %w", err)
	}
	cs, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating clientset: %w", err)
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

	s, err := confsuite.NewConformanceTestSuite(opts)
	if err != nil {
		return nil, fmt.Errorf("constructing conformance suite: %w", err)
	}

	// Custom setup: Configure Applier without invoking suite.Setup() which includes
	// TLS bootstrap and namespace constraints that aren't relevant for this POC.
	setupApplier(s, opts.ManifestFS, gatewayClassName)

	return s, nil
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

// ApplyBaseManifests applies base manifests (gateway, backend) with auto-cleanup.
// Manifests are applied via the suite's Applier; t.Cleanup handles teardown.
func ApplyBaseManifests(t *testing.T, suite *confsuite.ConformanceTestSuite, manifests []string) {
	t.Helper()
	for _, manifest := range manifests {
		suite.Applier.MustApplyWithCleanup(t, suite.Client, suite.TimeoutConfig, manifest, true)
	}
}
